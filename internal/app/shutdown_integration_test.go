//go:build integration

package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
)

// The parent holds this row lock until HTTP admission stops. The real request
// transaction must then finish with its own context, or roll back on fencing.
func registerShutdownTestRoutes(r *gin.Engine, pool *pgxpool.Pool, id string, writer *audit.PolicyEvalWriter) {
	r.GET("/drain-write", func(c *gin.Context) {
		ctx := c.Request.Context()
		_, err := pool.Exec(ctx, "INSERT INTO f0_effects(id) VALUES($1)", id+"_drain_entered")
		if err != nil {
			c.Status(http.StatusInternalServerError)
			return
		}
		tx, err := pool.Begin(ctx)
		if err == nil {
			defer func() { _ = tx.Rollback(context.Background()) }()
			_, err = tx.Exec(ctx, "SELECT n FROM f0_effects WHERE id=$1 FOR UPDATE", id+"_gate")
		}
		if err == nil {
			_, err = tx.Exec(ctx, "UPDATE f0_effects SET n=n+1 WHERE id=$1", id+"_drain_effect")
		}
		if err == nil {
			err = tx.Commit(ctx)
		}
		if err != nil {
			markerCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, _ = pool.Exec(markerCtx, "INSERT INTO f0_effects(id) VALUES($1)", id+"_drain_cancelled")
			c.Status(http.StatusServiceUnavailable)
			return
		}
		writer.Write(audit.PolicyEvalEntry{ActorID: 1, ResourceType: "shutdown_test", ResourceID: id + "_drain_audit", Action: "write", Result: true})
		c.Status(http.StatusOK)
	})
}

type shutdownResponse struct {
	status int
	err    error
}

func beginBlockedShutdownWrite(t *testing.T, id, expected string) (*singletonProcess, pgx.Tx, <-chan shutdownResponse) {
	t.Helper()
	_, err := singletonTestPool.Exec(context.Background(), "INSERT INTO f0_effects(id) VALUES($1),($2)", id+"_gate", id+"_drain_effect")
	require.NoError(t, err)
	gate, err := singletonTestPool.Begin(context.Background())
	require.NoError(t, err)
	t.Cleanup(func() { _ = gate.Rollback(context.Background()) })
	_, err = gate.Exec(context.Background(), "SELECT n FROM f0_effects WHERE id=$1 FOR UPDATE", id+"_gate")
	require.NoError(t, err)
	owner := startSingletonProcessExpected(t, id, expected)
	require.Eventually(t, owner.ready, 5*time.Second, 20*time.Millisecond)
	response := make(chan shutdownResponse, 1)
	go func() {
		resp, err := (&http.Client{Timeout: 40 * time.Second}).Get(fmt.Sprintf("http://127.0.0.1:%d/drain-write", owner.port))
		result := shutdownResponse{err: err}
		if err == nil {
			result.status = resp.StatusCode
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
		response <- result
	}()
	require.Eventually(t, func() bool { return shutdownMarkerExists(t, id+"_drain_entered") }, time.Second, 10*time.Millisecond)
	return owner, gate, response
}

func shutdownMarkerExists(t *testing.T, id string) bool {
	t.Helper()
	var exists bool
	require.NoError(t, singletonTestPool.QueryRow(context.Background(), "SELECT EXISTS(SELECT 1 FROM f0_effects WHERE id=$1)", id).Scan(&exists))
	return exists
}

func TestAppSIGTERMDrainsInFlightTransactionBeforeCancellingRequests(t *testing.T) {
	const id = "shutdown_normal"
	owner, gate, response := beginBlockedShutdownWrite(t, id, "normal")
	require.NoError(t, owner.cmd.Process.Signal(syscall.SIGTERM))
	require.Eventually(t, func() bool { return !owner.ready() }, time.Second, 10*time.Millisecond)
	select {
	case <-owner.done:
		t.Fatal("normal shutdown exited before the admitted transaction could drain")
	default:
	}
	backgroundAfter := f0Effects(t, id)
	time.Sleep(40 * time.Millisecond)
	require.Equal(t, backgroundAfter, f0Effects(t, id), "signals must stop background scheduling while admitted HTTP drains")
	require.NoError(t, gate.Commit(context.Background()))
	select {
	case result := <-response:
		require.NoError(t, result.err)
		require.Equal(t, http.StatusOK, result.status, "an admitted request must retain its context through normal SIGTERM draining")
	case <-time.After(3 * time.Second):
		t.Fatal("admitted transaction did not finish during normal draining")
	}
	owner.wait(t)
	require.Equal(t, 1, f0Effects(t, id+"_drain_effect"), "the admitted real PostgreSQL transaction must commit once")
	require.Equal(t, 1, f0Effects(t, id+"_drain_audit"), "the audit consumer must stay alive until admitted HTTP producers finish")
	require.False(t, shutdownMarkerExists(t, id+"_drain_cancelled"))
}

func TestAppOwnershipLossDuringSIGTERMDrainStillCancelsRequest(t *testing.T) {
	const id = "shutdown_loss_during_drain"
	owner, gate, response := beginBlockedShutdownWrite(t, id, "lost")
	require.NoError(t, owner.cmd.Process.Signal(syscall.SIGTERM))
	require.Eventually(t, func() bool { return !owner.ready() }, time.Second, 10*time.Millisecond)
	var terminated int
	require.NoError(t, singletonTestPool.QueryRow(context.Background(), "SELECT count(*) FROM (SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND application_name='zhuzhao:singleton') s").Scan(&terminated))
	require.Equal(t, 1, terminated, "only the private fixture's singleton session is terminated")
	owner.wait(t)
	require.True(t, shutdownMarkerExists(t, id+"_drain_cancelled"), "ownership loss must cancel a request even after normal draining started")
	require.Equal(t, 0, f0Effects(t, id+"_drain_effect"))
	require.NoError(t, gate.Commit(context.Background()))
	select {
	case result := <-response:
		require.True(t, result.err != nil || result.status != http.StatusOK)
	case <-time.After(time.Second):
		t.Fatal("fenced request connection remained open")
	}
}

func TestAppSIGTERMDrainDeadlineCancelsUnfinishedTransaction(t *testing.T) {
	const id = "shutdown_drain_deadline"
	owner, gate, response := beginBlockedShutdownWrite(t, id, "deadline")
	require.NoError(t, owner.cmd.Process.Signal(syscall.SIGTERM))
	owner.waitWithin(t, 35*time.Second)
	require.True(t, shutdownMarkerExists(t, id+"_drain_cancelled"), "the drain deadline must cancel context-aware unfinished database work")
	require.Equal(t, 0, f0Effects(t, id+"_drain_effect"))
	require.NoError(t, gate.Commit(context.Background()))
	select {
	case result := <-response:
		require.True(t, result.err != nil || result.status != http.StatusOK)
	case <-time.After(time.Second):
		t.Fatal("request connection remained open after the drain deadline")
	}
}

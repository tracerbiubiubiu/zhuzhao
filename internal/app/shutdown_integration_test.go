//go:build integration

package app

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net"
	"net/http"
	"syscall"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/singleton"
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

func TestAppLostOwnerLeavesDurableAuditForSuccessor(t *testing.T) {
	ctx := context.Background()
	redisContainer, err := testcontainers.Run(ctx,
		"redis:8.2.11-alpine3.22@sha256:da23c2478319ceb4903857d77a2fd7aa5137d9e9d4c8165dafbacedac6403e95",
		testcontainers.WithExposedPorts("6379/tcp"),
		testcontainers.WithWaitStrategy(wait.ForListeningPort("6379/tcp")),
	)
	testcontainers.CleanupContainer(t, redisContainer)
	require.NoError(t, err)
	host, err := redisContainer.Host(ctx)
	require.NoError(t, err)
	port, err := redisContainer.MappedPort(ctx, "6379/tcp")
	require.NoError(t, err)
	addr := net.JoinHostPort(host, port.Port())
	t.Setenv("ZZ_F0_REDIS_ADDR", addr)
	t.Setenv("ZZ_F0_HOLD_FLUSH", "1")
	redis := goredis.NewClient(&goredis.Options{Addr: addr})
	t.Cleanup(func() { _ = redis.Close() })
	for _, duringDrain := range []bool{false, true} {
		t.Run(fmt.Sprintf("during_drain_%t", duringDrain), func(t *testing.T) {
			require.NoError(t, redis.Del(ctx, "audit:policy_eval:processing").Err())
			id := "shutdown_audit_" + rand.Text()
			var owner *singletonProcess
			if duringDrain {
				var gate pgx.Tx
				owner, gate, _ = beginBlockedShutdownWrite(t, id, "lost")
				defer func() { _ = gate.Rollback(ctx) }()
			} else {
				owner = startSingletonProcessExpected(t, id, "lost")
				require.Eventually(t, owner.ready, 5*time.Second, 20*time.Millisecond)
			}
			require.NoError(t, redis.RPush(ctx, "audit:policy_eval:processing", fmt.Sprintf(`{"resource_id":%q}`, id+"_durable")).Err())
			if duringDrain {
				require.NoError(t, owner.cmd.Process.Signal(syscall.SIGTERM))
				require.Eventually(t, func() bool { return !owner.ready() }, time.Second, 10*time.Millisecond)
			}
			var terminated int
			require.NoError(t, singletonTestPool.QueryRow(ctx, "SELECT count(*) FROM (SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND application_name='zhuzhao:singleton') s").Scan(&terminated))
			require.Equal(t, 1, terminated)
			owner.wait(t)
			require.False(t, shutdownMarkerExists(t, id+"_durable"), "a lost owner must skip final PG flush")
			remaining, err := redis.LLen(ctx, "audit:policy_eval:processing").Result()
			require.NoError(t, err)
			require.EqualValues(t, 1, remaining)

			successor := startSingletonProcess(t, id+"_successor")
			require.Eventually(t, successor.ready, 5*time.Second, 20*time.Millisecond)
			require.NoError(t, successor.cmd.Process.Signal(syscall.SIGTERM))
			successor.wait(t)
			require.Equal(t, 1, f0Effects(t, id+"_durable"))
			remaining, err = redis.LLen(ctx, "audit:policy_eval:processing").Result()
			require.NoError(t, err)
			require.Zero(t, remaining)
		})
	}
}

// Both workers try to update fixture rows locked by the parent. The domain
// hook uses Background while the detached worker inherits the real HTTP
// owner marker, after its original request has ended.
func registerOwnerLifetimeTestRoute(r *gin.Engine, pool *pgxpool.Pool, id string, events *domainevent.HookRegistry) <-chan struct{} {
	run := func(ctx context.Context, name string) {
		if _, err := pool.Exec(ctx, "INSERT INTO f0_effects(id) VALUES($1)", id+name+"_entered"); err != nil {
			return
		}
		_, err := pool.Exec(ctx, "UPDATE f0_effects SET n=n+1 WHERE id=$1", id+name+"_effect")
		if err != nil && ctx.Err() != nil {
			markerCtx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			_, _ = pool.Exec(markerCtx, "INSERT INTO f0_effects(id) VALUES($1)", id+name+"_cancelled")
		}
	}
	events.Subscribe(domainevent.BizTicket, "f0.owner-lifetime", domainevent.HookFunc(func(ctx context.Context, _ domainevent.DomainEvent) {
		run(ctx, "_hook")
	}))
	detachedDone := make(chan struct{})
	r.POST("/owner-work", func(c *gin.Context) {
		requestCtx := c.Request.Context()
		go func() {
			defer close(detachedDone)
			<-requestCtx.Done()
			run(singleton.Detach(requestCtx), "_detached")
		}()
		events.Dispatch(context.Background(), domainevent.DomainEvent{BizType: domainevent.BizTicket, BizID: id})
		c.Status(http.StatusNoContent)
	})
	return detachedDone
}

func TestAppOwnerLifetimeFencesDomainAndDetachedHTTPWork(t *testing.T) {
	for _, loseOwner := range []bool{false, true} {
		t.Run(fmt.Sprintf("lose_owner_%t", loseOwner), func(t *testing.T) {
			ctx := context.Background()
			t.Setenv("ZZ_F0_OWNER_HOOKS", "1")
			id := "shutdown_hooks_" + rand.Text()
			expected := "normal"
			if loseOwner {
				expected = "lost"
			}
			owner, requestGate, response := beginBlockedShutdownWrite(t, id, expected)
			defer func() { _ = requestGate.Rollback(ctx) }()
			hookEffect, detachedEffect := id+"_hook_effect", id+"_detached_effect"
			_, err := singletonTestPool.Exec(ctx, "INSERT INTO f0_effects(id) VALUES($1),($2)", hookEffect, detachedEffect)
			require.NoError(t, err)
			workerGate, err := singletonTestPool.Begin(ctx)
			require.NoError(t, err)
			defer func() { _ = workerGate.Rollback(ctx) }()
			_, err = workerGate.Exec(ctx, "SELECT n FROM f0_effects WHERE id IN ($1,$2) FOR UPDATE", hookEffect, detachedEffect)
			require.NoError(t, err)
			client := &http.Client{Timeout: time.Second}
			resp, err := client.Post(fmt.Sprintf("http://127.0.0.1:%d/owner-work", owner.port), "", nil)
			require.NoError(t, err)
			require.NoError(t, resp.Body.Close())
			require.Equal(t, http.StatusNoContent, resp.StatusCode)
			require.Eventually(t, func() bool {
				return shutdownMarkerExists(t, id+"_hook_entered") && shutdownMarkerExists(t, id+"_detached_entered")
			}, time.Second, 10*time.Millisecond)

			require.NoError(t, owner.cmd.Process.Signal(syscall.SIGTERM))
			require.Eventually(t, func() bool { return !owner.ready() }, time.Second, 10*time.Millisecond)
			if loseOwner {
				var terminated int
				require.NoError(t, singletonTestPool.QueryRow(ctx, "SELECT count(*) FROM (SELECT pg_terminate_backend(pid) FROM pg_stat_activity WHERE datname=current_database() AND application_name='zhuzhao:singleton') s").Scan(&terminated))
				require.Equal(t, 1, terminated)
				require.Eventually(t, func() bool { return shutdownMarkerExists(t, id+"_hook_cancelled") }, 3*time.Second, 10*time.Millisecond,
					"the bound hook must observe owner loss despite dispatch from Background")
				require.Eventually(t, func() bool { return shutdownMarkerExists(t, id+"_detached_cancelled") }, 3*time.Second, 10*time.Millisecond,
					"detached HTTP work must observe owner loss through BaseContext")
				owner.wait(t)
				require.Zero(t, f0Effects(t, hookEffect))
				require.Zero(t, f0Effects(t, detachedEffect))
				return
			}

			require.NoError(t, workerGate.Commit(ctx))
			require.Eventually(t, func() bool {
				return f0Effects(t, hookEffect) == 1 && f0Effects(t, detachedEffect) == 1
			}, time.Second, 10*time.Millisecond)
			require.NoError(t, requestGate.Commit(ctx))
			select {
			case result := <-response:
				require.NoError(t, result.err)
				require.Equal(t, http.StatusOK, result.status)
			case <-time.After(3 * time.Second):
				t.Fatal("admitted transaction did not finish during normal draining")
			}
			owner.wait(t)
			require.False(t, shutdownMarkerExists(t, id+"_hook_cancelled"))
			require.False(t, shutdownMarkerExists(t, id+"_detached_cancelled"))
		})
	}
}

//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type c05IAMQuery struct {
	conn *pgx.Conn
	inTx bool
	sql  string
	args []any
}

type c05IAMTrace struct {
	mu      sync.Mutex
	queries []c05IAMQuery
}

func (tr *c05IAMTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if repository.IAMGuardActive(ctx) {
		tr.mu.Lock()
		defer tr.mu.Unlock()
		tr.queries = append(tr.queries, c05IAMQuery{conn, conn.PgConn().TxStatus() == 'T', strings.ToLower(data.SQL), data.Args})
	}
	return ctx
}

func (*c05IAMTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func (tr *c05IAMTrace) take() []c05IAMQuery {
	tr.mu.Lock()
	defer tr.mu.Unlock()
	queries := tr.queries
	tr.queries = nil
	return queries
}

func assertC05IAMCredentialOrder(t *testing.T, queries []c05IAMQuery, actorID, patID int64, receiptWrite bool) {
	t.Helper()
	var account, pat, receipt int = -1, -1, -1
	for i, q := range queries {
		switch {
		case strings.Contains(q.sql, "select status,") && strings.Contains(q.sql, "session_epoch"):
			require.Equal(t, []any{actorID}, q.args)
			account = i
		case strings.Contains(q.sql, "from personal_access_tokens"):
			require.Equal(t, []any{patID, actorID}, q.args)
			pat = i
		case strings.Contains(q.sql, "from user_session_operations") && receipt < 0:
			receipt = i
		}
	}
	require.NotEqual(t, -1, receipt, "the original operation is read inside the guard")
	require.True(t, queries[receipt].inTx)
	require.GreaterOrEqual(t, account, 0, "read the actor's current DB account and session facts inside the guard")
	require.Less(t, account, receipt, "check the credential before reading any receipt")
	require.True(t, queries[account].inTx)
	require.Same(t, queries[receipt].conn, queries[account].conn)
	if patID != 0 {
		require.GreaterOrEqual(t, pat, 0, "recheck the exact initiating PAT in the guard")
		require.Less(t, pat, receipt)
		require.True(t, queries[pat].inTx)
		require.Same(t, queries[receipt].conn, queries[pat].conn)
	} else {
		require.Equal(t, -1, pat, "a current Bearer session uses only its session credential facts")
	}
	wroteReceipt := false
	for _, q := range queries {
		if strings.Contains(q.sql, "insert into user_session_operations") {
			wroteReceipt = true
			require.True(t, q.inTx)
			require.Same(t, queries[receipt].conn, q.conn)
		}
		if !receiptWrite && strings.Contains(q.sql, "session_projection_intents") {
			require.True(t, strings.HasPrefix(strings.TrimSpace(q.sql), "select "))
			require.True(t, q.inTx)
			require.Same(t, queries[receipt].conn, q.conn)
		}
	}
	require.Equal(t, receiptWrite, wroteReceipt)
}

func newC05IAMTracePool(t *testing.T, ctx context.Context) (*c05IAMTrace, *pgxpool.Pool) {
	t.Helper()
	trace := &c05IAMTrace{}
	cfg := testPool.Config().Copy()
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return trace, pool
}

func assertC05IAMReceipt(t *testing.T, result *httptest.ResponseRecorder, operationID, command string, target, epoch int64) service.UserOperationResultView {
	t.Helper()
	epochHTTPCode(t, result, http.StatusOK, 0)
	require.Equal(t, operationID, result.Header().Get("X-Operation-ID"))
	require.Equal(t, "completed", result.Header().Get("X-Operation-Status"))
	var response struct {
		Data service.UserOperationResultView
	}
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &response))
	require.Equal(t, operationID, response.Data.OperationID)
	require.Equal(t, command, response.Data.Command)
	require.Equal(t, itoa(target), response.Data.TargetUserID)
	require.Equal(t, "completed", response.Data.Status)
	require.Equal(t, epoch, response.Data.ResultEpoch)
	require.Equal(t, "completed", response.Data.ProjectionState)
	require.False(t, response.Data.ProjectionPending)
	return response.Data
}

func TestC05IAMGuard_CurrentCredentialsAndOriginalReceipt(t *testing.T) {
	e := newEpochRegressionAuth(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	claims, err := e.mgr.ParseAccessToken(e.actor.AccessToken)
	require.NoError(t, err)
	actorID := claims.UserID
	pat, token, err := service.NewPatService(repository.NewPatRepo(testPool), slog.Default()).Create(ctx, actorID, "C05 current PAT", 1)
	require.NoError(t, err)
	trace, pool := newC05IAMTracePool(t, ctx)
	router := e.router(t, pool, e.auth)

	t.Run("current session commits status and reads its receipt", func(t *testing.T) {
		const operationID = "c05-iam-current-status"
		body := fmt.Appendf(nil, `{"user_id":%q,"status":0}`, itoa(e.user.ID))
		command := epochRequest(ctx, router, http.MethodPost, "/api/v1/users/status", e.actor.AccessToken, operationID, body)
		epochHTTPCode(t, command, 200, 0)
		assertC05IAMCredentialOrder(t, trace.take(), actorID, 0, true)
		before, err := e.repo.SecuritySnapshot(ctx, e.user.ID)
		require.NoError(t, err)
		require.Zero(t, before.Status)
		require.EqualValues(t, 2, before.SessionEpoch)
		result := epochRequest(ctx, router, http.MethodGet, "/api/v1/users/status/result?operation_id="+operationID, e.actor.AccessToken, "", nil)
		view := assertC05IAMReceipt(t, result, operationID, service.SessionCmdUserDisable, e.user.ID, 2)
		require.NotNil(t, view.AccountStatus)
		require.Zero(t, *view.AccountStatus)
		assertC05IAMCredentialOrder(t, trace.take(), actorID, 0, false)
		after, err := e.repo.SecuritySnapshot(ctx, e.user.ID)
		require.NoError(t, err)
		require.Equal(t, before, after, "lookup preserves the target's account security facts")
	})

	t.Run("current PAT keeps its own lifecycle across a password reset", func(t *testing.T) {
		const resetID = "c05-iam-current-reset"
		const password = "C05SyntheticReset#2026"
		body := fmt.Appendf(nil, `{"user_id":%q,"password":%q}`, itoa(actorID), password)
		command := epochRequest(ctx, router, http.MethodPost, "/api/v1/users/password/reset", token, resetID, body)
		epochHTTPCode(t, command, 200, 0)
		assertC05IAMCredentialOrder(t, trace.take(), actorID, pat.ID, true)
		account, err := e.repo.SecuritySnapshot(ctx, actorID)
		require.NoError(t, err)
		require.EqualValues(t, 2, account.SessionEpoch)
		result := epochRequest(ctx, router, http.MethodGet, "/api/v1/users/status/result?operation_id="+resetID, token, "", nil)
		view := assertC05IAMReceipt(t, result, resetID, service.SessionCmdUserResetPwd, actorID, 2)
		require.Nil(t, view.AccountStatus)
		require.NotContains(t, result.Body.String(), password)
		require.NotContains(t, result.Body.String(), token)
		assertC05IAMCredentialOrder(t, trace.take(), actorID, pat.ID, false)

		const deleteID = "c05-iam-current-delete"
		targetBefore, err := e.repo.SecuritySnapshot(ctx, e.user.ID)
		require.NoError(t, err)
		body = fmt.Appendf(nil, `{"user_id":%q}`, itoa(e.user.ID))
		command = epochRequest(ctx, router, http.MethodPost, "/api/v1/users/delete", token, deleteID, body)
		epochHTTPCode(t, command, 200, 0)
		assertC05IAMCredentialOrder(t, trace.take(), actorID, pat.ID, true)
		deleted, err := e.repo.SecuritySnapshot(ctx, e.user.ID)
		require.NoError(t, err)
		require.True(t, deleted.Deleted)
		require.Equal(t, targetBefore.SessionEpoch+1, deleted.SessionEpoch)
	})
}

func TestC05IAMGuard_SelfReceiptAfterCurrentLogin(t *testing.T) {
	e := newEpochRegressionAuth(t)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	old := e.login(t, "EpochOld#2026", "self-receipt")
	const operationID = "c05-iam-current-self-receipt"
	newPassword := "C05SyntheticSelf#2026"
	body := fmt.Appendf(nil, `{"old_password":"EpochOld#2026","new_password":%q,"device_id":"self-receipt"}`, newPassword)
	command := epochRequest(ctx, e.engine, http.MethodPost, "/api/v1/auth/password/update", old.AccessToken, operationID, body)
	epochHTTPCode(t, command, 200, 0)
	current := e.login(t, newPassword, "self-receipt")
	path := "/api/v1/users/status/result?operation_id=" + operationID
	t.Run("current account has its original result", func(t *testing.T) {
		trace, pool := newC05IAMTracePool(t, ctx)
		result := epochRequest(ctx, e.router(t, pool, e.auth), http.MethodGet, path, current.AccessToken, "", nil)
		view := assertC05IAMReceipt(t, result, operationID, service.SessionCmdChangePassword, e.user.ID, 2)
		require.Nil(t, view.AccountStatus)
		require.NotContains(t, result.Body.String(), newPassword)
		require.NotContains(t, result.Body.String(), current.AccessToken)
		assertC05IAMCredentialOrder(t, trace.take(), e.user.ID, 0, false)
		account, err := e.repo.SecuritySnapshot(ctx, e.user.ID)
		require.NoError(t, err)
		require.EqualValues(t, 2, account.SessionEpoch)
		require.EqualValues(t, 2, account.ProjectionEpoch)
	})
	t.Run("administrative access preserves self receipt privacy", func(t *testing.T) {
		result := epochRequest(ctx, e.engine, http.MethodGet, path, e.actor.AccessToken, "", nil)
		epochHTTPCode(t, result, http.StatusOK, 0)
		require.Equal(t, operationID, result.Header().Get("X-Operation-ID"))
		require.Equal(t, "not_found", result.Header().Get("X-Operation-Status"))
		var response struct {
			Data service.UserOperationResultView
		}
		require.NoError(t, json.Unmarshal(result.Body.Bytes(), &response))
		require.Equal(t, service.UserOperationResultView{OperationID: operationID, Status: "not_found"}, response.Data)
	})
}

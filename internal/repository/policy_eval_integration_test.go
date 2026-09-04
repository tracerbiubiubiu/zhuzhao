//go:build integration

// B11① 判定日志集成验证（真 PG + miniredis）：writer 管道端到端（hook→channel→
// Redis List→批量落库）+ request_id 三列（policy_evaluation_logs.trace_id /
// ticket_events.request_id / audit_logs.request_id）同键贯通（03 §3.4）。
package repository_test

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/audit"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestPolicyEvalPipelineEndToEnd(t *testing.T) {
	ctx := context.Background()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	repo := repository.NewAuditLogRepo(testPool)
	w := audit.NewPolicyEvalWriter(audit.PolicyEvalConfig{BatchSize: 10, FlushInterval: 30 * time.Millisecond}, rdb, repo, nil)
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	w.Start(runCtx)

	rid := "req-e2e-policy-eval"
	w.Write(audit.PolicyEvalEntry{
		ActorID: 42, ActorRoles: []string{"user"}, ResourceType: "ticket",
		ResourceID: "99", Action: "update", Result: false, Reason: "denied", TraceID: rid,
	})

	deadline := time.Now().Add(3 * time.Second)
	var n int
	for time.Now().Before(deadline) {
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM policy_evaluation_logs WHERE actor_id=$1 AND trace_id=$2 AND result=false AND reason='denied'`,
			42, rid).Scan(&n))
		if n == 1 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	require.Equal(t, 1, n, "判定行应经管道落入 policy_evaluation_logs")

	var roles []string
	var reason string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT actor_role_codes, reason FROM policy_evaluation_logs WHERE trace_id=$1`, rid).Scan(&roles, &reason))
	require.Equal(t, []string{"user"}, roles)
	require.Equal(t, "denied", reason)

	// 消费后 List/processing 清空（不残留）
	require.Eventually(t, func() bool {
		a, _ := rdb.LLen(ctx, "audit:policy_eval").Result()
		b, _ := rdb.LLen(ctx, "audit:policy_eval:processing").Result()
		return a == 0 && b == 0
	}, 2*time.Second, 20*time.Millisecond)
}

func TestRequestIDColumnsEndToEnd(t *testing.T) {
	// 1) audit_logs.request_id：repo.Create 带值 → 落列；空值 → NULL
	auditRepo := repository.NewAuditLogRepo(testPool)
	require.NoError(t, auditRepo.Create(context.Background(), &model.AuditLog{
		Username: "b11it", Method: "GET", Path: "/api/v1/it/audit", StatusCode: 200,
		Duration: 5, RequestID: "req-audit-cols",
	}))
	var rid *string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT request_id FROM audit_logs WHERE username='b11it' ORDER BY id DESC LIMIT 1`).Scan(&rid))
	require.NotNil(t, rid)
	require.Equal(t, "req-audit-cols", *rid)

	// 2) ticket_events.request_id：CreateEventTx 公共路径从 ctx 取（有/无 ctx 值两种）
	uid := seedBuiltinUser(t, "b11evt"+baSuffix())
	orgID := seedBuiltinOrg(t, "b11org"+baSuffix())
	var ticketID int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
		INSERT INTO tickets (type_code, title, status, created_by, org_id, org_path)
		VALUES ('incident', 'B11① request_id 列验证', 'open', $1, $2, $3::ltree) RETURNING id`, uid, orgID, "b11p"+baSuffix()).Scan(&ticketID))
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM tickets WHERE id=$1`, ticketID) })

	ticketRepo := repository.NewTicketRepo(testPool)
	ctxWith := reqid.With(context.Background(), "req-event-cols")
	require.NoError(t, ticketRepo.CreateEvent(ctxWith, &model.TicketEvent{
		TicketID: ticketID, UserID: uid, Action: "comment"}))
	require.NoError(t, ticketRepo.CreateEvent(context.Background(), &model.TicketEvent{
		TicketID: ticketID, UserID: uid, Action: "note"}))

	var evRID *string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT request_id FROM ticket_events WHERE ticket_id=$1 AND action='comment'`, ticketID).Scan(&evRID))
	require.NotNil(t, evRID)
	require.Equal(t, "req-event-cols", *evRID)
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT request_id FROM ticket_events WHERE ticket_id=$1 AND action='note'`, ticketID).Scan(&evRID))
	require.Nil(t, evRID, "无 ctx 值时应为 NULL")
}

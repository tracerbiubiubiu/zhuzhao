// Package reqid request_id 的 request context 载体（03-audit-l2 §3.4 全链路关联）。
// 中间件注入（gin ctx + request ctx 双写），service/repo 层经 ctx 读取——
// slog = audit_logs = ticket_events = policy_evaluation_logs = taskrunner job_runs 同键。
package reqid

import "context"

type ctxKey struct{}

// With 将 request_id 注入 context（幂等：后写覆盖）。
func With(ctx context.Context, rid string) context.Context {
	return context.WithValue(ctx, ctxKey{}, rid)
}

// From 读取 request_id；未注入返回空串（调用方按可空列处理）。
func From(ctx context.Context) string {
	rid, _ := ctx.Value(ctxKey{}).(string)
	return rid
}

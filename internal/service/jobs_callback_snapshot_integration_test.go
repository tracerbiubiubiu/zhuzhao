//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// W0b（二十批⑥）：SK 泄露/taskrunner 被控场景下，可用一个已存在的 task_id
// 换一个已注册动作执行（凭证行 action 保留首记，但 Execute 按回调 body 分发）。
// 修复：Claim 后比对行快照与回调 body 的 action/params，不匹配 → 409 拒执行。
func TestJobsCallback_SnapshotMismatchRejected(t *testing.T) {
	_, err := testPool.Exec(context.Background(), `TRUNCATE job_submissions RESTART IDENTITY`)
	require.NoError(t, err)
	ctx := context.Background()

	var legitCalls, attackCalls int32
	reg := jobs.NewRegistry()
	reg.Register("legit_action", jobs.HandlerFunc(func(ctx context.Context, params json.RawMessage) error {
		atomic.AddInt32(&legitCalls, 1)
		return nil
	}))
	reg.Register("attack_action", jobs.HandlerFunc(func(ctx context.Context, params json.RawMessage) error {
		atomic.AddInt32(&attackCalls, 1)
		return nil
	}))

	repo := repository.NewJobSubmissionRepo(testPool)
	svc := service.NewJobsCallbackService(reg, repo, slog.Default())

	const taskID = "task-snapshot-service"

	// 首记：合法链路执行过 legit_action（终态 succeeded）
	out, msg := svc.Execute(ctx, service.CallbackInput{
		TaskID: taskID, Action: "legit_action", Params: json.RawMessage(`{"k":"v"}`),
		Actor: "zhuzhao", SourceIP: "10.0.0.1",
	})
	require.NotEqual(t, service.CallbackRetryable, out, "首记执行不应可重试失败: %s", msg)
	assert.Equal(t, int32(1), atomic.LoadInt32(&legitCalls))

	// 挪用：同 task_id 携 attack_action（已注册）——不得执行、不得 2xx 受理
	out2, msg2 := svc.Execute(ctx, service.CallbackInput{
		TaskID: taskID, Action: "attack_action", Params: json.RawMessage(`{"retention_days":0}`),
		Actor: "attacker", SourceIP: "1.2.3.4",
	})
	assert.Equal(t, int32(0), atomic.LoadInt32(&attackCalls), "快照不匹配的动作不得被执行")
	assert.Equal(t, service.CallbackNonRetryable, out2, "快照不匹配须 409 不可重试（当前返回 %v/%s）", out2, msg2)
}

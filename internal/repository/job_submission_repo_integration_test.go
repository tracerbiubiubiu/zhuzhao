//go:build integration

package repository_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func resetJobSubmissions(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `TRUNCATE job_submissions RESTART IDENTITY`)
	require.NoError(t, err)
}

// P1-1 回归：并发同 task_id 调用 ClaimCallbackRow → 恰好 1 个取得执行权。
// 修复前（EnsureCallbackRow 两段式）8 并发全部放行、副作用执行 8 次。
func TestJobSubmissionRepo_ClaimCallbackRowConcurrentSingleWinner(t *testing.T) {
	resetJobSubmissions(t)
	ctx := context.Background()
	repo := repository.NewJobSubmissionRepo(testPool)

	const n = 8
	const taskID = "task-concurrent-single-winner"

	claimed := make([]bool, n)
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start // 栅栏对齐，最大化并发窗口
			_, c, err := repo.ClaimCallbackRow(ctx, taskID, "sync_users", "E100001", "10.0.0.1", `{"since":"2026-01-01"}`)
			claimed[i] = c
			errs[i] = err
		}(i)
	}
	close(start)
	wg.Wait()

	winners := 0
	for i := 0; i < n; i++ {
		require.NoErrorf(t, errs[i], "claim #%d 返回错误", i)
		if claimed[i] {
			winners++
		}
	}
	assert.Equal(t, 1, winners, "并发同 task_id 必须恰好 1 个取得执行权（回归 8/8 全放行）")

	// 落库终态：唯一胜者行 status=running
	var status string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT status FROM job_submissions WHERE task_id = $1`, taskID).Scan(&status))
	assert.Equal(t, repository.JobStatusRunning, status)
}

// P1-1 顺序语义：覆盖「首次抢占 / 在途拦截 / 陈旧自愈 / 终态拦截 / 失败重试」全分支。
func TestJobSubmissionRepo_ClaimCallbackRowSequentialSemantics(t *testing.T) {
	resetJobSubmissions(t)
	ctx := context.Background()
	repo := repository.NewJobSubmissionRepo(testPool)

	// 1) 无行（cron 触发）→ 补录并取得执行权
	row, claimed, err := repo.ClaimCallbackRow(ctx, "task-seq-1", "sync_users", "E100002", "10.0.0.2", "{}")
	require.NoError(t, err)
	assert.True(t, claimed, "首次到达应取得执行权")
	assert.Equal(t, repository.JobStatusRunning, row.Status)

	// 2) 在途 running 且未超 10 分钟 → 拒绝（他人正在执行）
	row, claimed, err = repo.ClaimCallbackRow(ctx, "task-seq-1", "sync_users", "E100002", "10.0.0.2", "{}")
	require.NoError(t, err)
	assert.False(t, claimed, "在途 running 未超时不得重复认领")
	require.NotNil(t, row, "未抢到时应返回已存在行供调用方映射响应")
	assert.Equal(t, repository.JobStatusRunning, row.Status)

	// 3) 陈旧 running（claimed_at 置为 11 分钟前）→ 视为进程崩溃，允许自愈重认领
	_, err = testPool.Exec(ctx,
		`UPDATE job_submissions SET claimed_at = NOW() - INTERVAL '11 minutes' WHERE task_id = $1`, "task-seq-1")
	require.NoError(t, err)
	row, claimed, err = repo.ClaimCallbackRow(ctx, "task-seq-1", "sync_users", "E100002", "10.0.0.2", "{}")
	require.NoError(t, err)
	assert.True(t, claimed, "running 超 10 分钟视为陈旧，允许重认领（崩溃自愈）")
	assert.Equal(t, repository.JobStatusRunning, row.Status)

	// 4) 已 succeeded（终态）→ 幂等拦截
	require.NoError(t, repo.MarkSucceeded(ctx, "task-seq-1"))
	row, claimed, err = repo.ClaimCallbackRow(ctx, "task-seq-1", "sync_users", "E100002", "10.0.0.2", "{}")
	require.NoError(t, err)
	assert.False(t, claimed, "succeeded 终态必须幂等拦截")
	assert.Equal(t, repository.JobStatusSucceeded, row.Status)

	// 5) failed（非终态）→ 允许重试
	_, claimed, err = repo.ClaimCallbackRow(ctx, "task-seq-2", "sync_users", "E100002", "10.0.0.2", "{}")
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, repo.MarkFailed(ctx, "task-seq-2", "boom"))
	row, claimed, err = repo.ClaimCallbackRow(ctx, "task-seq-2", "sync_users", "E100002", "10.0.0.2", "{}")
	require.NoError(t, err)
	assert.True(t, claimed, "failed 非终态，允许重试")
	assert.Equal(t, repository.JobStatusRunning, row.Status)
	assert.Empty(t, row.Error, "重认领应清空上次 error")
}

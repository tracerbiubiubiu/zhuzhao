//go:build integration

// B5-5 cron 最小件补测（2026-10-21 G1）：Run 全链四态——成功/失败/panic 的
// Status 记录 + advisory lock 单实例互斥（同名双调度器并发 max=1）。
// lock 查询失败分支（PG 层故障注入）不做——注入面为破坏连接池，代价不成比例。
package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// cronOkJob 计数成功。
type cronOkJob struct{ calls atomic.Int64 }

func (j *cronOkJob) Handle(context.Context, json.RawMessage) error { j.calls.Add(1); return nil }

// cronFailJob 恒失败。
type cronFailJob struct{}

func (*cronFailJob) Handle(context.Context, json.RawMessage) error { return errors.New("boom") }

// cronPanicJob 恒 panic——进程存活 + Status 记录 panic 语义。
type cronPanicJob struct{}

func (*cronPanicJob) Handle(context.Context, json.RawMessage) error { panic("kaboom") }

// cronSlowJob 慢任务（300ms）+ 并发在飞峰值统计——advisory lock 互斥断言用。
type cronSlowJob struct{ running, maxRunning atomic.Int64 }

func (j *cronSlowJob) Handle(context.Context, json.RawMessage) error {
	cur := j.running.Add(1)
	for {
		old := j.maxRunning.Load()
		if cur <= old || j.maxRunning.CompareAndSwap(old, cur) {
			break
		}
	}
	time.Sleep(300 * time.Millisecond)
	j.running.Add(-1)
	return nil
}

// waitStatus 轮询直到 cond 为真或超时（deadline 5s——tick 50ms 足够余量）。
func waitStatus(t *testing.T, s *service.CronScheduler, cond func(service.CronStatus) bool) service.CronStatus {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		st := s.Status()
		if cond(st) {
			return st
		}
		if time.Now().After(deadline) {
			t.Fatal("等待 CronStatus 条件超时")
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func TestCronScheduler_RunRecordsStatus(t *testing.T) {
	t.Run("成功路径：LastSuccessAt 就位且失败面为零", func(t *testing.T) {
		job := &cronOkJob{}
		s := service.NewCronScheduler(testPool, "t-cron-ok-"+uniqueSuffix(), job, 50*time.Millisecond, nil)
		ctx, cancel := context.WithCancel(context.Background())
		go s.Run(ctx)
		st := waitStatus(t, s, func(x service.CronStatus) bool { return !x.LastSuccessAt.IsZero() })
		cancel()
		assert.True(t, st.LastFailureAt.IsZero(), "纯成功任务失败面应保持零值")
		assert.Empty(t, st.LastError)
		assert.GreaterOrEqual(t, job.calls.Load(), int64(1))
	})

	t.Run("失败路径：LastFailureAt/LastError 记录且下 tick 重试", func(t *testing.T) {
		s := service.NewCronScheduler(testPool, "t-cron-fail-"+uniqueSuffix(), &cronFailJob{}, 50*time.Millisecond, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go s.Run(ctx)
		st := waitStatus(t, s, func(x service.CronStatus) bool { return !x.LastFailureAt.IsZero() })
		assert.Equal(t, "boom", st.LastError)
		assert.True(t, st.LastSuccessAt.IsZero(), "恒失败任务不应记成功")
		// 可重入契约：下个 tick 自然重试——失败时间持续推进
		first := st.LastFailureAt
		waitStatus(t, s, func(x service.CronStatus) bool { return x.LastFailureAt.After(first) })
	})

	t.Run("panic 路径：进程存活且末次错误为 panic 语义", func(t *testing.T) {
		s := service.NewCronScheduler(testPool, "t-cron-panic-"+uniqueSuffix(), &cronPanicJob{}, 50*time.Millisecond, nil)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go s.Run(ctx) // 若 panic 兜底失效，本测试进程直接退出——测试本身即断言
		st := waitStatus(t, s, func(x service.CronStatus) bool { return !x.LastFailureAt.IsZero() })
		assert.Equal(t, "job panicked (see error log)", st.LastError)
	})
}

// TestCronScheduler_AdvisoryLockExclusion 同名双调度器互斥：慢任务（300ms）+
// 快 tick（50ms）跑 1.2s——advisory lock 生效则并发在飞恒 ≤1、零交叠。
func TestCronScheduler_AdvisoryLockExclusion(t *testing.T) {
	job := &cronSlowJob{}
	name := "t-cron-lock-" + uniqueSuffix()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go service.NewCronScheduler(testPool, name, job, 50*time.Millisecond, nil).Run(ctx)
	go service.NewCronScheduler(testPool, name, job, 50*time.Millisecond, nil).Run(ctx)

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) && job.maxRunning.Load() == 0 {
		time.Sleep(20 * time.Millisecond)
	}
	require.NotZero(t, job.maxRunning.Load(), "任务应在期限内至少执行一次")
	time.Sleep(1200 * time.Millisecond) // 双调度器交叠窗口充分暴露
	cancel()
	assert.LessOrEqual(t, job.maxRunning.Load(), int64(1), "同名任务并发在飞应恒 ≤1（advisory lock 单实例互斥）")
}

package service

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// B5-5 cron 最小件补测（2026-10-21 G1）——记录语义与默认值纯单测；
// Run 全链（lock/成功/失败/panic）见 cron_scheduler_integration_test.go。
func TestCronStatus_RecordSemantics(t *testing.T) {
	s := &CronScheduler{name: "unit"}

	t.Run("零值=从未成功", func(t *testing.T) {
		st := s.Status()
		assert.True(t, st.LastSuccessAt.IsZero())
		assert.True(t, st.LastFailureAt.IsZero())
		assert.Empty(t, st.LastError)
		assert.Equal(t, time.Duration(0), st.LastDuration)
	})

	t.Run("recordSuccess 后 LastSuccessAt/LastDuration 就位", func(t *testing.T) {
		before := time.Now()
		s.recordSuccess(1500 * time.Millisecond)
		st := s.Status()
		require.False(t, st.LastSuccessAt.IsZero())
		assert.True(t, !st.LastSuccessAt.Before(before), "时间不早于调用前")
		assert.Equal(t, 1500*time.Millisecond, st.LastDuration)
		assert.True(t, st.LastFailureAt.IsZero(), "成功不清失败面（末次错误留档）")
	})

	t.Run("recordFailure 记录末次错误并截断 200", func(t *testing.T) {
		s.recordFailure(errors.New(strings.Repeat("x", 250)))
		st := s.Status()
		require.False(t, st.LastFailureAt.IsZero())
		assert.Len(t, st.LastError, 200, "超 200 截断（对齐判定日志 reason 口径）")

		s.recordFailure(errors.New("boom"))
		assert.Equal(t, "boom", s.Status().LastError, "末次错误覆盖前次")

		s.recordFailure(nil)
		assert.Empty(t, s.Status().LastError, "nil 错误记空串不留旧值")
	})

	t.Run("并发读写安全（-race 面值）", func(t *testing.T) {
		var wg sync.WaitGroup
		for i := 0; i < 50; i++ {
			wg.Add(2)
			go func() { defer wg.Done(); s.recordSuccess(time.Millisecond) }()
			go func() { defer wg.Done(); _ = s.Status() }()
		}
		wg.Wait()
	})
}

func TestNewCronScheduler_DefaultsAndLockKey(t *testing.T) {
	a := NewCronScheduler(nil, "job-a", nil, 0, nil)
	b := NewCronScheduler(nil, "job-a", nil, -time.Second, nil)
	c := NewCronScheduler(nil, "job-b", nil, time.Second, nil)

	assert.Equal(t, 30*time.Second, a.tick, "tick<=0 取默认 30s")
	assert.Equal(t, 30*time.Second, b.tick)
	assert.NotNil(t, a.logger, "nil logger 取 slog.Default()")

	assert.Equal(t, a.lockKey, b.lockKey, "同名任务锁键恒定（跨实例同键=单实例互斥前提）")
	assert.NotEqual(t, a.lockKey, c.lockKey, "异名任务锁键不同")
}

func TestCronPanicError(t *testing.T) {
	assert.Equal(t, "job panicked (see error log)", errPanic.Error())
}

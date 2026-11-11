// notifyThrottle 同单连发节流单测（B6-5 补遗——06 号 §5.2）。
package ticket

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNotifyThrottle(t *testing.T) {
	t.Run("首次放行", func(t *testing.T) {
		th := newNotifyThrottle(60 * time.Second)
		assert.False(t, th.shouldSkip("ticket", "1", "ticket.assigned"), "首次应放行")
	})

	t.Run("窗口内重复拒", func(t *testing.T) {
		th := newNotifyThrottle(60 * time.Second)
		th.shouldSkip("ticket", "1", "ticket.assigned") // 首次
		assert.True(t, th.shouldSkip("ticket", "1", "ticket.assigned"), "窗口内同单同动作应拒")
	})

	t.Run("不同动作互不干扰", func(t *testing.T) {
		th := newNotifyThrottle(60 * time.Second)
		th.shouldSkip("ticket", "1", "ticket.assigned")
		assert.False(t, th.shouldSkip("ticket", "1", "ticket.closed"), "同单不同动作应放行")
	})

	t.Run("不同单互不干扰", func(t *testing.T) {
		th := newNotifyThrottle(60 * time.Second)
		th.shouldSkip("ticket", "1", "ticket.assigned")
		assert.False(t, th.shouldSkip("ticket", "2", "ticket.assigned"), "不同单同动作应放行")
	})

	t.Run("窗口过期后放行", func(t *testing.T) {
		th := newNotifyThrottle(1 * time.Millisecond)
		th.shouldSkip("ticket", "1", "ticket.assigned")
		time.Sleep(5 * time.Millisecond)
		assert.False(t, th.shouldSkip("ticket", "1", "ticket.assigned"), "窗口过期后应放行")
	})

	t.Run("并发安全", func(t *testing.T) {
		th := newNotifyThrottle(60 * time.Second)
		th.shouldSkip("ticket", "1", "ticket.assigned") // 预填
		// 并发 100 次 shouldSkip——不应 panic（sync.Mutex 保护）
		done := make(chan bool, 100)
		for i := 0; i < 100; i++ {
			go func() {
				th.shouldSkip("ticket", "1", "ticket.assigned")
				done <- true
			}()
		}
		for i := 0; i < 100; i++ {
			<-done
		}
	})
}

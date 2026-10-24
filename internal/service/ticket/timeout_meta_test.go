// validateTransitionMeta / escalationPlan 纯函数单测（S10 B10-4——零 DB 直测）。
package ticket

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestValidateTransitionMeta(t *testing.T) {
	states := []string{"open", "assigned", "closed"}

	t.Run("合法：timeout + escalation 全形态", func(t *testing.T) {
		meta := `{"assigned":{"timeout_minutes":60,"escalation":{"notify":["assignee","supervisor"],"reminder_interval_minutes":30,"max_reminders":2}},"open":{"timeout_minutes":1440}}`
		assert.NoError(t, validateTransitionMeta(states, json.RawMessage(meta)))
	})

	t.Run("负向：键越域/非对象/非正分钟/未知 notify 目标/间隔 < 节流窗", func(t *testing.T) {
		for _, tc := range []struct {
			name string
			meta string
		}{
			{"键不在 states", `{"pending_verify":{"timeout_minutes":60}}`},
			{"非对象", `["x"]`},
			{"分钟非正", `{"open":{"timeout_minutes":0}}`},
			{"notify 未知目标", `{"open":{"timeout_minutes":5,"escalation":{"notify":["manager"]}}}`},
			{"间隔小于节流窗", `{"open":{"timeout_minutes":5,"escalation":{"reminder_interval_minutes":1}}}`},
			{"段结构非法", `{"open":{"timeout_minutes":5,"escalation":"x"}}`},
		} {
			err := validateTransitionMeta(states, json.RawMessage(tc.meta))
			require.Error(t, err, tc.name)
			var ee *errcode.Error
			require.ErrorAs(t, err, &ee, tc.name)
			assert.Equal(t, errcode.ErrInvalidParams.Code, ee.Code, tc.name)
		}
	})
}

func TestEscalationPlan(t *testing.T) {
	t.Run("无段=单发 assignee", func(t *testing.T) {
		notify, interval, max := escalationPlan(nil)
		assert.Equal(t, []string{"assignee"}, notify)
		assert.Equal(t, defaultReminderIntervalMinutes, interval)
		assert.Equal(t, 1, max)
	})
	t.Run("有段取段值（缺省补 240/3）", func(t *testing.T) {
		notify, interval, max := escalationPlan(&stateMetaEscalation{Notify: []string{"supervisor"}})
		assert.Equal(t, []string{"supervisor"}, notify)
		assert.Equal(t, defaultReminderIntervalMinutes, interval)
		assert.Equal(t, defaultMaxReminders, max)
	})
	t.Run("全自定义", func(t *testing.T) {
		i, m := 15, 5
		_, interval, max := escalationPlan(&stateMetaEscalation{Notify: []string{"assignee", "supervisor"}, ReminderIntervalMinutes: &i, MaxReminders: &m})
		assert.Equal(t, 15, interval)
		assert.Equal(t, 5, max)
	})
}

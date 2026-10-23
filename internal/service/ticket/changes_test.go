package ticket

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// TestBuildChanges D20 changes[] 键级 diff 回归锚（外审 F-S6-1-3——45fa90e
// 实现零测试；本锚锁当前契约：Key+NewVal 恒填、custom_data 仅键、空请求空集）。
// 注：OldVal（from）契约偏差（F-S6-1-7）未在本锚语义内——拍板后随批改。
func TestBuildChanges(t *testing.T) {
	title := "新标题"
	priority := 3
	desc := "新描述"

	t.Run("title+priority 变更→两条含新值", func(t *testing.T) {
		changes := buildChanges(&model.UpdateTicketRequest{Title: &title, Priority: &priority})
		require.Len(t, changes, 2)

		byKey := map[string]any{}
		for _, c := range changes {
			byKey[c.Key] = c.NewVal
		}
		assert.Equal(t, title, byKey["title"])
		assert.Equal(t, priority, byKey["priority"])
	})

	t.Run("description 变更→键+新值", func(t *testing.T) {
		changes := buildChanges(&model.UpdateTicketRequest{Description: &desc})
		require.Len(t, changes, 1)
		assert.Equal(t, "description", changes[0].Key)
		assert.Equal(t, desc, changes[0].NewVal)
	})

	t.Run("custom_data 变更→仅键（载荷体积不进 changes）", func(t *testing.T) {
		changes := buildChanges(&model.UpdateTicketRequest{CustomData: []byte(`{"a":1}`)})
		require.Len(t, changes, 1)
		assert.Equal(t, "custom_data", changes[0].Key)
		assert.Nil(t, changes[0].NewVal, "custom_data 只记键不记值")
	})

	t.Run("空请求→空集", func(t *testing.T) {
		assert.Empty(t, buildChanges(&model.UpdateTicketRequest{}))
	})
}

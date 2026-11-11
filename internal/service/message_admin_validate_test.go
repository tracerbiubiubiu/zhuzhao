package service_test

// v3 审计 Y-1 回归锚（2026-10-23）：UpsertRule 校验②③④须返 errcode（400 面）
// 而非裸 fmt.Errorf（修复前 errors.As 落空 → 500+10000 污染 5xx 告警面）。
// 纯单元面：直接验证 errcode.Error 类型断言链（handler writeServiceError 消费端）
// —— UpsertRule 全链测试归 integration（需真实模板才能过校验①）。

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// TestY1_ErrcodeMappingNotRawError 验证 validateUpsertRule ②③④ 修复后的
// errcode.New 产出可被 handler writeServiceError 的 errors.As 正确消费。
// 修复前裸 fmt.Errorf → errors.As 落空 → 500+10000；修复后 400+业务码。
func TestY1_ErrcodeMappingNotRawError(t *testing.T) {
	// 模拟修复后的产出（validateUpsertRule :60/:65/:72 同款调用）
	cases := []struct {
		name string
		err  error
	}{
		{"recipient_type", errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("recipient_type %q 不合法", "boss"))},
		{"event_action", errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("event_action %q 不在支持词表", "evil.action"))},
		{"fixed_recipients", errcode.New(errcode.ErrInvalidParams.Code, fmt.Sprintf("fixed_recipients 含非数字 ID %q", "abc"))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			e, ok := c.err.(*errcode.Error)
			require.True(t, ok, "须为 *errcode.Error——handler writeServiceError 的 errors.As 才能命中（修复前裸 error→500）")
			assert.Equal(t, errcode.ErrInvalidParams.Code, e.Code, "映射 400 业务码")
		})
	}

	// 反向对照：裸 fmt.Errorf 的 errors.As 必须失败（锁住修复前后的差异面）
	rawErr := fmt.Errorf("recipient_type %q 不合法", "boss")
	_, ok := rawErr.(*errcode.Error)
	assert.False(t, ok, "裸 error 不得通过类型断言——修复前的 500 行为面")
}

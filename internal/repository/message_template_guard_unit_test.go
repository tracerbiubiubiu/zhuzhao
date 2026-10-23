// 删除守卫纯函数单测（外审 F3-1-1：CI 可跑的守卫覆盖——DB 全链在 integration 测试）。
package repository

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestCheckDeleteGuard(t *testing.T) {
	t.Run("系统模板拒删 92503", func(t *testing.T) {
		err := checkDeleteGuard(true)
		require.Error(t, err)
		assert.Equal(t, errcode.ErrMsgTplSystem.Code, err.(*errcode.Error).Code)
	})
	t.Run("非系统放行", func(t *testing.T) {
		require.NoError(t, checkDeleteGuard(false))
	})
}

package handler_test

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// fakeCronProvider 可控状态提供者（CronObserveFlagger 可选面——本件带 Observe）。
type fakeCronProvider struct {
	status service.CronStatus
}

func (f *fakeCronProvider) Status() service.CronStatus { return f.status }
func (f *fakeCronProvider) Observe() bool              { return true }

// TestCronStatus_View 观测面视图（审计缺口批——端点零测试）：
// Snapshot→视图字段映射 + 观察模式透出 + nil 注册表空面不 panic。
func TestCronStatus_View(t *testing.T) {
	reg := service.NewCronRegistry()
	successAt := time.Now().Add(-time.Minute)
	reg.Register("timeout_scanner", &fakeCronProvider{status: service.CronStatus{
		LastSuccessAt: successAt, LastError: "", LastDuration: 1200 * time.Millisecond,
	}})
	reg.Register("bad.job", &fakeCronProvider{status: service.CronStatus{LastError: "boom"}})

	h := handler.NewSystemHandler(reg)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	h.CronStatus(c)

	require.Equal(t, 200, w.Code)
	var out struct {
		Code int `json:"code"`
		Data struct {
			Jobs map[string]struct {
				LastError      string  `json:"last_error"`
				LastDurationMs int64   `json:"last_duration_ms"`
				LastSuccessAt  *string `json:"last_success_at"`
				Observe        *bool   `json:"observe"`
			} `json:"jobs"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &out))
	assert.Equal(t, 0, out.Code)

	scanner := out.Data.Jobs["timeout_scanner"]
	require.NotNil(t, scanner.LastSuccessAt, "成功时间透出")
	assert.Equal(t, int64(1200), scanner.LastDurationMs, "耗时毫秒化")
	require.NotNil(t, scanner.Observe, "观察模式标记透出（CronObserveFlagger 可选面）")
	assert.True(t, *scanner.Observe)

	bad := out.Data.Jobs["bad.job"]
	assert.Equal(t, "boom", bad.LastError)
	assert.Nil(t, bad.LastSuccessAt, "零值时间不出（指针形态）")

	t.Run("nil 注册表=空面不 panic", func(t *testing.T) {
		h2 := handler.NewSystemHandler(nil)
		w2 := httptest.NewRecorder()
		c2, _ := gin.CreateTestContext(w2)
		h2.CronStatus(c2)
		assert.Equal(t, 200, w2.Code)
	})
}

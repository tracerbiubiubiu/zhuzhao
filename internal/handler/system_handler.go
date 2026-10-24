// SystemHandler 系统内务端点（S10 B10-4——cron 可观测面，10 号 A.5）。
package handler

import (
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// CronJobView cron 任务状态快照（GET /system/cron-status 出站形态）。
type CronJobView struct {
	Name           string     `json:"name"`
	LastSuccessAt  *time.Time `json:"last_success_at,omitempty"`
	LastFailureAt  *time.Time `json:"last_failure_at,omitempty"`
	LastError      string     `json:"last_error,omitempty"`
	LastDurationMs int64      `json:"last_duration_ms"`
	// Observe 超时扫描观察模式标记（仅 timeout_scan 带——运维确认首跑无异常后关闭）
	Observe *bool `json:"observe,omitempty"`
}

// SystemHandler 系统内务处理器。
type SystemHandler struct {
	cronReg *service.CronRegistry
}

// NewSystemHandler 创建系统内务处理器（注册表实例与 NewApp 装配共享——wire_gen 传入）。
func NewSystemHandler(cronReg *service.CronRegistry) *SystemHandler {
	return &SystemHandler{cronReg: cronReg}
}

// CronStatus GET /api/v1/system/cron-status
//
//	@Summary	cron 任务状态（只读——上次成功/失败/耗时；超时扫描含观察模式标记）
//	@Tags		system
//	@Produce	json
//	@Success	200	{object}	response.Response
//	@Router		/api/v1/system/cron-status [get]
func (h *SystemHandler) CronStatus(c *gin.Context) {
	jobs := map[string]CronJobView{}
	if h.cronReg != nil {
		for name, p := range h.cronReg.Snapshot() {
			st := p.Status()
			v := CronJobView{Name: name, LastError: st.LastError, LastDurationMs: st.LastDuration.Milliseconds()}
			if !st.LastSuccessAt.IsZero() {
				t := st.LastSuccessAt
				v.LastSuccessAt = &t
			}
			if !st.LastFailureAt.IsZero() {
				t := st.LastFailureAt
				v.LastFailureAt = &t
			}
			if f, ok := p.(service.CronObserveFlagger); ok {
				ob := f.Observe()
				v.Observe = &ob
			}
			jobs[name] = v
		}
	}
	response.OK(c, gin.H{"jobs": jobs})
}

package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk/compat"
	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
)

// CompatHandler 存量协议兼容层端点（/compat 路由组，utils aksk/compat 验签）。
// 预接线形态（2026-10-14）：Echo 为 runbook §5.2 上线前向量比对的实弹靶——
// 内网真实调用场景就位前即可独立完成「线协议吻合 + caller 归因」验证；业务
// 端点随场景就位后同组并列（纪律：组内实质单端点或幂等/无副作用，
// standards §3-11 豁免——路由组只隔离组外，防不了组内互相重放）。
type CompatHandler struct{}

func NewCompatHandler() *CompatHandler { return &CompatHandler{} }

// Echo 存量协议兼容层验证端点（预接线）。
//
//	@Summary		存量协议兼容层验证端点
//	@Description	AK/SK 验签通过即回显 caller/request_id——存量调用场景就位前按 runbook §5.2 完成线协议比对的判定靶；无业务副作用。存量 x-auth-* 签名（非 JWT），鉴权走 compat 中间件
//	@Tags			compat
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	response.Response
//	@Failure		401	{object}	response.Response
//	@Router			/compat/echo [post]
func (h *CompatHandler) Echo(c *gin.Context) {
	response.OK(c, gin.H{
		"caller": c.GetString(compat.ContextKeyCaller),
		// 键名与 AccessLogger/审计行同源（middleware.RequestID 写入；无导出常量，
		// 与 casbin.go 同以字面量取用）
		"request_id": c.GetString("request_id"),
	})
}

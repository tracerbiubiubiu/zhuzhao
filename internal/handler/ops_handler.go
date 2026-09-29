package handler

import (
	"context"
	"sort"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
)

// PanicListFn panic 聚合查询出口（函数 Port——app 层适配 repo；handler 不 import repository：架构红线）
type PanicListFn func(ctx context.Context, page, pageSize int) (list any, total int64, err error)

// OpsHandler P4-8 P2 小件：panic 聚合查询 + 只读路由对账 + /metrics 指标。
type OpsHandler struct {
	panics PanicListFn
	// 对账闭包注入（装配处组合 router.Routes/LoadBoundAPIs/gateway 前缀——
	// router 包 import handler.Deps，handler 不得反向 import router 防循环）
	reconcile func(ctx context.Context) []string
}

func NewOpsHandler(pr PanicListFn, reconcile func(ctx context.Context) []string) *OpsHandler {
	return &OpsHandler{panics: pr, reconcile: reconcile}
}

// ListPanics GET /audit/panics（聚合列表——admin 面）
func (h *OpsHandler) ListPanics(c *gin.Context) {
	page, pageSize := pageOf(c)
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	list, total, err := h.panics(c.Request.Context(), page, pageSize)
	if err != nil {
		writeServiceError(c, err)
		return
	}
	response.OK(c, gin.H{"list": list, "total": total, "page": page, "page_size": pageSize})
}

// Reconcile GET /audit/reconcile（运行时路由↔menu_apis 双向对账——启动 fail-fast
// 的查询版：missing_binding=有路由无绑定 / dead_binding=有绑定无路由）
func (h *OpsHandler) Reconcile(c *gin.Context) {
	out := h.reconcile(c.Request.Context())
	if out == nil {
		out = []string{}
	}
	sort.Strings(out)
	response.OK(c, gin.H{"gaps": out, "gap_count": len(out), "checked_at": time.Now().Format(time.RFC3339)})
}

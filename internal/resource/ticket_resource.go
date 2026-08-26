package ticket

import (
	"context"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

// Resource 工单资源，实现 resource.Resource 接口
// 本文件为 Phase 2a Step 1 骨架，fail-closed 实现。
// 具体鉴权逻辑 (Authorize / GetFilter) 将在后续步骤实现。
type Resource struct {
	// 后续步骤将注入 TicketRepo 或 Service
}

// NewResource 创建工单资源实例
func NewResource() *Resource {
	return &Resource{}
}

// Code 返回资源编码，用于 Registry 路由
func (r *Resource) Code() string {
	return "ticket"
}

// Name 返回资源名称（用于日志/调试）
func (r *Resource) Name() string {
	return "工单 (Ticket)"
}

// Actions 返回工单资源支持的所有动作
func (r *Resource) Actions() []string {
	return []string{
		"list",
		"create",
		"read",
		"update",
		"delete",
		"assign",
		"close",
		"comment",
		"note",
	}
}

// Authorize 资源级鉴权（含可见性 + 属主 + 操作权）
//
// Phase 2a Step 1 骨架：默认返回 false (fail-closed)，语义未就绪。
// 逻辑实现见 Phase 2a Step 2。
func (r *Resource) Authorize(_ context.Context, req resource.AuthorizeRequest) (bool, error) {
	// Phase 2a: 骨架 fail-closed
	// Step 2 将根据 req.Action 区分 read/update/close/assign 等语义
	return false, fmt.Errorf("ticket resource authorize: not implemented (fail-closed by default)")
}

// GetFilter 返回列表行级过滤条件（scope 可见性）
//
// Phase 2a Step 1 骨架：返回空 Filter（无数据可见），语义未就绪。
// 逻辑实现见 Phase 2a Step 2。
func (r *Resource) GetFilter(_ context.Context, _ int64, action string) (resource.Filter, error) {
	// Phase 2a: 骨架 fail-closed (可见性=空集)
	// Step 2 将根据 scope (assigned/group/all) 生成 WHERE 子句
	return resource.Filter{
		Where: "1 = 0", // 无数据可见
		Args:  nil,
	}, nil
}

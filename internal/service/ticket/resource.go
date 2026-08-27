package ticket

import (
	"context"
	"errors"
	"strconv"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

// ticketRepo 资源层需要的工单仓储接口（解耦 resource 与具体 repo）
type ticketRepo interface {
	GetByID(ctx context.Context, id int64) (*model.Ticket, error)
}

// Resource 工单资源，实现 resource.Resource 接口。
// 替换 Step 1 骨架（internal/resource/ticket_resource.go）的真实现。
// 2a 范围：assigned scope（L2 = created_by OR assigned_to）+ 属主判断（L3）。
type Resource struct {
	repo  ticketRepo
	scope ScopeResolver
}

// NewResource 创建工单资源实例（由 NewTicketService 调用，注册到 Registry）
func NewResource(repo ticketRepo, scope ScopeResolver) *Resource {
	return &Resource{repo: repo, scope: scope}
}

// Code 返回资源编码
func (r *Resource) Code() string { return "ticket" }

// Name 返回资源名称
func (r *Resource) Name() string { return "工单 (Ticket)" }

// Actions 返回工单资源支持的动作
func (r *Resource) Actions() []string {
	return []string{"list", "create", "read", "update", "delete", "assign", "close", "comment", "note"}
}

// Authorize 资源级鉴权（L2 可见性 → L3 属主 + canOperate）
// admin/superadmin bypass L2/L3；L1 路由级仍走 Casbin。
func (r *Resource) Authorize(ctx context.Context, req resource.AuthorizeRequest) (bool, error) {
	// admin bypass
	if HasRole(req.Roles, "admin") || HasRole(req.Roles, "superadmin") {
		return true, nil
	}

	// create 恒 true（路由级已校验 ticket:create）
	if req.Action == "create" || req.Action == "list" {
		return true, nil
	}

	// 需要具体工单的操作：解析 ResourceID
	ticketID, err := strconv.ParseInt(req.ResourceID, 10, 64)
	if err != nil || ticketID <= 0 {
		return false, errcode.ErrInvalidParams
	}

	ticket, err := r.repo.GetByID(ctx, ticketID)
	if err != nil {
		// 工单不存在 → (false, nil)，Service 层转 404
		if errors.Is(err, errcode.ErrTicketNotFound) {
			return false, nil
		}
		// 其他 DB 错误 → 原样上抛，Service 层转 500
		return false, err
	}

	// L2 可见性（scope=assigned 即仅属主可见）
	if !r.canRead(req.UserID, ticket) {
		return false, nil // Service 层转 404
	}

	// L3 属主 + canOperate（动作权）
	return r.canOperate(req.UserID, req.Action, ticket), nil
}

// GetFilter 列表行级过滤（2a：assigned scope）
func (r *Resource) GetFilter(_ context.Context, userID int64, _ string) (resource.Filter, error) {
	return resource.Filter{
		Where: "(created_by = $1 OR assigned_to = $1)",
		Args:  []interface{}{userID},
	}, nil
}

// canRead 2a 可见性判断：created_by == userID || assigned_to == userID
func (r *Resource) canRead(userID int64, ticket *model.Ticket) bool {
	if ticket.CreatedBy == userID {
		return true
	}
	if ticket.AssignedTo != nil && *ticket.AssignedTo == userID {
		return true
	}
	return false
}

// canOperate 2a 动作级权限：
//
//	read/comment = canRead；update = 创建人或处理人；close = 处理人；
//	assign = 仅 admin bypass（2a 暂不开放主管分派）；
//	delete = 仅 admin bypass。
//	属主命中≠能做所有动作（assign/delete 属主也不放行）。
func (r *Resource) canOperate(userID int64, action string, ticket *model.Ticket) bool {
	isCreator := ticket.CreatedBy == userID
	isAssignee := ticket.AssignedTo != nil && *ticket.AssignedTo == userID

	switch action {
	case "read", "comment":
		return true // canRead 已通过
	case "note":
		// 2a：创建人或处理人可写内部备注（与 assigned scope 对齐）
		return isCreator || isAssignee
	case "update":
		// 2a：创建人或处理人可更新；2b 收为仅创建人
		return isCreator || isAssignee
	case "close":
		// 2a：处理人可关闭（创建人也可，对齐 09-ticket §5.4）
		return isAssignee || isCreator
	case "assign", "delete":
		// 2a：仅 admin bypass（主管分派 2b scope=group/all 扩展）
		return false
	default:
		return false
	}
}

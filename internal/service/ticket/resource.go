package ticket

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

// ticketRepo 资源层需要的工单仓储接口（解耦 resource 与具体 repo）
type ticketRepo interface {
	GetByID(ctx context.Context, id int64) (*model.Ticket, error)
}

// errDenied 哨兵错误：L3 canOperate 拒绝（工单可见但不允许该动作）。
// 与 (false, nil) 区分：后者 = 不可见/不存在 → Service 层转 404；
// errDenied = 可见但无权限 → Service 层转 403。
var errDenied = errors.New("ticket: operation denied")

// Resource 工单资源，实现 resource.Resource 接口。
// 2b 范围（09-ticket §5.2）：策略 B——L2 读 = 属主 ∨ 实体锚点透明读（org_path <@ ANY）；
// L3 写：update 仅创建人（RK-11 收窄）、close 创建人/处理人、assign/delete 仅 admin。
type Resource struct {
	repo     ticketRepo
	resolver ScopeResolver
}

// NewResource 创建工单资源实例（由 NewTicketService 调用，注册到 Registry）
func NewResource(repo ticketRepo, resolver ScopeResolver) *Resource {
	return &Resource{repo: repo, resolver: resolver}
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

	// 一次解析用户 scope（透明锚点 / scope 子树 / 全量），L2 与 L3 共用
	scope, err := r.resolver.ResolveScope(ctx, req.UserID)
	if err != nil {
		// 解析器 DB 错误上抛（Q3：→ 500/503），不与 404 混淆
		return false, err
	}

	// L2 可见性（策略 B ∪ scope 扩展）：属主 ∨ 全量 ∨ 路径命中
	if !isOwner(req.UserID, ticket) && !scope.AllScope &&
		!pathInAnchors(ticket.OrgPath, scope.ReadPaths()) {
		return false, nil // 不可见 → Service 层转 404
	}

	// L3 canOperate（动作权，读写分离）
	if !r.canOperate(ctx, scope, req.UserID, req.Action, ticket) {
		return false, errDenied // 可见但无权限 → Service 层转 403
	}
	return true, nil
}

// isOwner 属主判定：创建人或处理人
func isOwner(userID int64, ticket *model.Ticket) bool {
	if ticket.CreatedBy == userID {
		return true
	}
	return ticket.AssignedTo != nil && *ticket.AssignedTo == userID
}

// GetFilter 列表行级过滤（2b 策略 B，09-ticket §5.2）：
// 属主（created_by/assigned_to）∨ 实体锚点透明读（org_path <@ ANY 锚点）。
// 锚点为空（用户无组织归属）时第三支恒假，退化为 2a assigned 语义。
func (r *Resource) GetFilter(ctx context.Context, userID int64, _ string) (resource.Filter, error) {
	scope, err := r.resolver.ResolveScope(ctx, userID)
	if err != nil {
		return resource.Filter{}, err
	}
	// ticket_scope=all：列表全量（rbac-inheritance §4），仍受 L1 路由级 Casbin 约束
	if scope.AllScope {
		return resource.Filter{}, nil
	}
	paths := scope.ReadPaths()
	return resource.Filter{
		Where: `(created_by = $1 OR assigned_to = $1 OR org_path <@ ANY($2::ltree[]))`,
		Args:  []interface{}{userID, paths},
	}, nil
}

// pathInAnchors ltree 标签级前缀匹配：org_path 落在任一锚点子树内
func pathInAnchors(orgPath string, anchors []string) bool {
	for _, a := range anchors {
		if orgPath == a || strings.HasPrefix(orgPath, a+".") {
			return true
		}
	}
	return false
}

// canOperate 2b 动作级权限（09-ticket §5.2，读/写分离）：
//
//	read/comment = canRead（透明读可见即可读）；
//	note = 创建人/处理人（与 BK-1 内部备注读过滤对齐：写者 ⊆ 可见内部备注集合，
//	       透明读旁观者不可写；2c 扩 org admin/owner——见 09 §5.4 回标）；
//	update = 仅创建人（RK-11 收窄：透明读可见≠可改，处理人改单走 close/重新分派）；
//	close = 处理人或创建人；
//	assign = ticket_scope group/all 主管（2b-org 000012 后激活；此前仅 admin bypass）；
//	delete = 仅 admin bypass。
func (r *Resource) canOperate(ctx context.Context, scope *ResolvedScope, userID int64, action string, ticket *model.Ticket) bool {
	isCreator := ticket.CreatedBy == userID
	isAssignee := ticket.AssignedTo != nil && *ticket.AssignedTo == userID

	switch action {
	case "read", "comment":
		return true // L2 可见性已通过
	case "note":
		// 2b：与内部备注读可见集合一致（BK-1），透明读旁观者不可写
		return isCreator || isAssignee
	case "update":
		// 2b 收窄为仅创建人（RK-11：显式回归「处理人 update 应 403」）
		return isCreator
	case "close":
		// 处理人可关闭（创建人也可，对齐 09-ticket §5.4）
		return isAssignee || isCreator
	case "assign":
		// 2b-org 主管激活（09 §5.2）：ticket_scope∈{group,all} 且工单在其 scope 子树内
		return scope.AllScope || pathInAnchors(ticket.OrgPath, scope.ScopePaths)
	case "delete":
		// 仅 admin bypass
		return false
	default:
		return false
	}
}

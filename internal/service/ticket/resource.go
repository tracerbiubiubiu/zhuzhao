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

// OrgDelegationChecker 2c 组织委托判定（04 §4；实现于 service.OrgDelegationService）。
// 接口注入避免 ticket 包对 service 具体类型的依赖。
type OrgDelegationChecker interface {
	// IsOrgAdminOrOwner 工单所属 org 的组内 admin/owner（含 owner_user_ids 双轨）
	IsOrgAdminOrOwner(ctx context.Context, userID, orgID int64) (bool, error)
	// IsAncestorOwner 实体部门 owner 对子树的委托（D9）
	IsAncestorOwner(ctx context.Context, userID int64, ticketOrgID int64, ticketOrgPath string) (bool, error)
}

// Resource 工单资源，实现 resource.Resource 接口。
// 2b（09-ticket §5.2）：策略 B——L2 读 = 属主 ∨ 实体锚点透明读；
// 2c（04 §4.2）：L3 写加组织委托（org admin/owner + ancestor owner）。
type Resource struct {
	repo       ticketRepo
	resolver   ScopeResolver
	delegation OrgDelegationChecker
}

// NewResource 创建工单资源实例（由 NewTicketService 调用，注册到 Registry）
func NewResource(repo ticketRepo, resolver ScopeResolver, delegation OrgDelegationChecker) *Resource {
	return &Resource{repo: repo, resolver: resolver, delegation: delegation}
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

	// L2 可见性（策略 B ∪ scope 扩展）：属主 ∨ 全量 ∨ 路径命中 ∨ 组织委托。
	// 委托轴（BK-13）：project_isolated 强隔离下锚点消失，org admin/owner 与
	// ancestor owner 若不在 L2 放行，则 L3 委托永远走不到（Authorize 先 L2 后 L3），
	// D7–D9 语义失效——故委托命中同样视为可见；透明读下该分支为冗余超集，仅多两次
	// 索引查询且只发生在 404 路径上。
	visible := isOwner(req.UserID, ticket) || scope.AllScope ||
		pathInAnchors(ticket.OrgPath, scope.ReadPaths())
	if !visible {
		del, err := r.delegation.IsOrgAdminOrOwner(ctx, req.UserID, ticket.OrgID)
		if err != nil {
			return false, err
		}
		if !del {
			del, err = r.delegation.IsAncestorOwner(ctx, req.UserID, ticket.OrgID, ticket.OrgPath)
			if err != nil {
				return false, err
			}
		}
		visible = del
	}
	if !visible {
		return false, nil // 不可见 → Service 层转 404
	}

	// L3 canOperate（动作权，读写分离；2c 含组织委托）
	allowed, err := r.canOperate(ctx, scope, req.UserID, req.Action, ticket)
	if err != nil {
		return false, err // 委托判定 DB 错误 → 500（Q3）
	}
	if !allowed {
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
// 属主（created_by/assigned_to）∨ 实体锚点透明读（org_path <@ ANY 锚点）
// ∨ 组织委托（BK-13 委托轴，与 org_delegation 判定同语义）。
// 锚点为空（用户无组织归属）时锚点支恒假，退化为属主 ∪ 委托语义。
func (r *Resource) GetFilter(ctx context.Context, userID int64, _ string) (resource.Filter, error) {
	scope, err := r.resolver.ResolveScope(ctx, userID)
	if err != nil {
		return resource.Filter{}, err
	}
	// ticket_scope=all：列表全量（rbac-inheritance §4），仍受 L1 路由级 Casbin 约束；
	// 显式豁免（IW4——零值 Filter{} 已被 repo 哨兵拒绝）
	if scope.AllScope {
		return resource.Filter{Unscoped: true}, nil
	}
	paths := scope.ReadPaths()
	return resource.Filter{
		Where: `(created_by = $1 OR assigned_to = $1 OR org_path <@ ANY($2::ltree[]) OR ` +
			delegatedVisibilitySQL + `)`,
		Args: []interface{}{userID, paths},
	}, nil
}

// delegatedVisibilitySQL L2 委托轴（BK-13）：工单所属 org 的 owner（owner_user_ids）
// / org admin·owner / ancestor owner——与 org_delegation.IsOrgAdminOrOwner/
// IsAncestorOwner 同语义；org_id 为外层 tickets.org_id 关联列。
// 命中索引：organizations PK、user_orgs PK (user_id, org_id)、organizations.path GIN。
const delegatedVisibilitySQL = `EXISTS (
	SELECT 1 FROM organizations o
	WHERE o.id = org_id
	  AND (
	       $1 = ANY(o.owner_user_ids)
	    OR EXISTS (SELECT 1 FROM user_orgs du
	               WHERE du.org_id = o.id AND du.user_id = $1
	                 AND du.org_member_role IN ('admin', 'owner'))
	    OR EXISTS (SELECT 1 FROM organizations anc
	               WHERE anc.path @> o.path AND $1 = ANY(anc.owner_user_ids))
	  ))`

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
func (r *Resource) canOperate(ctx context.Context, scope *ResolvedScope, userID int64, action string, ticket *model.Ticket) (bool, error) {
	isCreator := ticket.CreatedBy == userID
	isAssignee := ticket.AssignedTo != nil && *ticket.AssignedTo == userID

	// 2c 组织委托（04 §4.2）：工单所属 org 的 admin/owner 或 ancestor owner。
	// 语义边界：委托管「本 org（含子树，ancestor owner）绑定的工单」——
	// 凭 vg_a 的 admin 身份不能改 vg_b 工单（org 不匹配），D11/R10 不受影响。
	delegated := false

	needDelegation := false
	switch action {
	case "read", "comment":
		return true, nil // L2 可见性已通过
	case "note":
		// 2b：读写集合一致（BK-1）；2c 扩 org admin/owner（09 §5.4 回标）
		if isCreator || isAssignee {
			return true, nil
		}
		needDelegation = true
	case "update":
		// 2b 仅创建人（RK-11）；2c + org admin·owner / ancestor owner
		if isCreator {
			return true, nil
		}
		needDelegation = true
	case "close":
		if isAssignee || isCreator {
			return true, nil
		}
		needDelegation = true
	case "assign":
		// 2b 主管（scope group/all 且工单在其子树）；2c + org admin·owner
		if scope.AllScope || pathInAnchors(ticket.OrgPath, scope.ScopePaths) {
			return true, nil
		}
		needDelegation = true
	case "delete":
		// 2b 仅 admin bypass；2c + org admin·owner / ancestor owner
		needDelegation = true
	default:
		return false, nil
	}

	if needDelegation {
		var err error
		delegated, err = r.delegation.IsOrgAdminOrOwner(ctx, userID, ticket.OrgID)
		if err != nil {
			return false, err
		}
		if !delegated {
			delegated, err = r.delegation.IsAncestorOwner(ctx, userID, ticket.OrgID, ticket.OrgPath)
			if err != nil {
				return false, err
			}
		}
	}
	return delegated, nil
}

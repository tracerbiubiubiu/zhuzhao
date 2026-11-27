package ticket

import (
	"context"
	"errors"
	"strconv"
	"strings"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// ticketRepo 资源层需要的工单仓储接口（解耦 resource 与具体 repo）
type ticketRepo interface {
	GetVisibleByID(ctx context.Context, id int64, filter resource.Filter) (*model.Ticket, error)
	MatchesLiveOrgPaths(ctx context.Context, orgID int64, paths []string) (bool, error)
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
	// HasOrgManagePermission 全局组织成员管理权（org:member；Create归属校验豁免）
	HasOrgManagePermission(ctx context.Context, userID int64) (bool, error)
}

// Resource 工单资源，实现 resource.Resource 接口。
// 2b（09-ticket §5.2）：策略 B——L2 读 = 属主 ∨ 实体锚点透明读；
// 2c（04 §4.2）：L3 写加组织委托（org admin/owner + ancestor owner）；
// S10 B10-1：读可见集新增抄送特例 OR 支（GetFilter + Authorize 可见段）。
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
	return []string{"list", "create", "read", "update", "delete", "assign", "close", "comment", "note", "transition", "transfer", "withdraw"}
}

// Authorize 资源级鉴权（L2 可见性 → L3 属主 + canOperate）
// admin/superadmin bypass L2/L3；L1 路由级仍走 Casbin。
func (r *Resource) Authorize(ctx context.Context, req resource.AuthorizeRequest) (bool, error) {
	// admin bypass
	if HasRole(req.Roles, "admin") || HasRole(req.Roles, "superadmin") {
		return true, nil
	}

	// create 恒 true（W1 后路由级语义：L1=页面 GET/按钮 POST 绑定——POST /tickets 挂
	// ticket_create_btn；归属约束在 service.Create 前置（IsInOrgBranch，W0b P0-5））
	if req.Action == "create" || req.Action == "list" {
		return true, nil
	}

	// 需要具体工单的操作：解析 ResourceID
	ticketID, err := strconv.ParseInt(req.ResourceID, 10, 64)
	if err != nil || ticketID <= 0 {
		return false, errcode.ErrInvalidParams
	}

	scope, err := r.resolver.ResolveScope(ctx, req.UserID)
	if err != nil {
		return false, err
	}
	ticket, err := r.repo.GetVisibleByID(ctx, ticketID, ticketReadFilter(req.UserID, scope))
	if errors.Is(err, errcode.ErrTicketNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
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

// GetFilter supplies the same type policy and scope as single-ticket authorization.
func (r *Resource) GetFilter(ctx context.Context, userID int64, _ string) (resource.Filter, error) {
	scope, err := r.resolver.ResolveScope(ctx, userID)
	if err != nil {
		return resource.Filter{}, err
	}
	return ticketReadFilter(userID, scope), nil
}

func ticketReadFilter(userID int64, scope *ResolvedScope) resource.Filter {
	return resource.Filter{Where: ticketReadSQL, Args: []any{userID, scope.ReadPaths(), scope.AllScope}}
}

// Only active approved role codes and explicit CC (never a free-form user field)
// can extend restricted reads beyond the ordinary ticket owners.
var ticketReadSQL = `(tickets.created_by=$1 OR tickets.assigned_to=$1 OR ` + ccVisibilitySQL + ` OR EXISTS (
    SELECT 1 FROM ticket_types policy WHERE policy.code=tickets.type_code AND (
        (policy.read_policy='inherit' AND ($3::boolean OR
            (tickets.org_path <@ ANY($2::ltree[]) AND ` + repository.LiveOrgPathAnchorSQL("tickets.org_id", "$2") + `) OR ` + delegatedVisibilitySQL + `))
        OR (policy.read_policy='role_restricted' AND policy.read_role_codes && ARRAY(
            ` + repository.EffectiveRoleCTE() + `SELECT code::text FROM roles WHERE id IN (SELECT id FROM expanded)
        ))
    )))`

// ccVisibilitySQL S10 B10-1 抄送特例支（拍板点 1 预确认「抄送即可见」）：
// cc 关系并入**读可见集**（列表/导出/关联查询全路径自动生效——GetFilter 本体增强
// 非新调用点，与 BK-21「显式调用零护栏」正交）；不进 canOperate（能看不能改）。
// 命中索引：idx_ticket_cc_user（000062）。
const ccVisibilitySQL = `EXISTS (
	SELECT 1 FROM ticket_cc
	WHERE ticket_id = tickets.id AND user_id = $1 AND role = 'cc')`

// delegatedVisibilitySQL L2 委托轴（BK-13）：工单所属 org 的 owner（owner_user_ids）
// / org admin·owner / ancestor owner——与 org_delegation.IsOrgAdminOrOwner/
// IsAncestorOwner 同语义；org_id 为外层 tickets.org_id 关联列。
// 命中索引：organizations PK、user_orgs PK (user_id, org_id)、organizations.path GIN。
var delegatedVisibilitySQL = `EXISTS (
	SELECT 1 FROM organizations o
	WHERE o.id = tickets.org_id AND (` + repository.OrgDelegationActiveSQL("o") + `)
	  AND (
	       $1 = ANY(o.owner_user_ids)
	    OR EXISTS (SELECT 1 FROM user_orgs du
	               WHERE du.org_id = o.id AND du.user_id = $1
	                 AND du.org_member_role IN ('admin', 'owner')
	                 AND (du.expires_at IS NULL OR du.expires_at > NOW()))
	    OR EXISTS (SELECT 1 FROM organizations anc
	               WHERE anc.id IN (` + repository.OrgAncestorIDsSQL("o") + `) AND (` + repository.OrgDelegationActiveSQL("anc") + `) AND $1 = ANY(anc.owner_user_ids))
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
//	transition = 处理人或创建人（S3a B3-1——与 close 同面；condition 细粒度
//	       在 canOperate 之上由 checkCondition 叠加——两层分离：L3 属主面 +
//	       transition_meta roles 面[可配 assignee/creator/org_admin/admin]）；
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
	case "transition": // S3a B3-1——L3 属主面与 close 同；condition 细粒度叠加在上层
		if isAssignee || isCreator {
			return true, nil
		}
		needDelegation = true
	case "withdraw":
		return isCreator, nil
	case "assign", "transfer":
		if action == "transfer" && isAssignee {
			return true, nil
		}
		// 2b 主管（scope group/all 且工单在其子树）；2c + org admin·owner
		if scope.AllScope {
			return true, nil
		}
		matched, err := r.livePathMatch(ctx, ticket, scope.ScopePaths)
		if err != nil {
			return false, err
		}
		if matched {
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

func (r *Resource) livePathMatch(ctx context.Context, ticket *model.Ticket, paths []string) (bool, error) {
	if !pathInAnchors(ticket.OrgPath, paths) {
		return false, nil
	}
	return r.repo.MatchesLiveOrgPaths(ctx, ticket.OrgID, paths)
}

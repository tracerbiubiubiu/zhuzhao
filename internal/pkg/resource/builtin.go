package resource

import (
	"context"
	"fmt"
)

// 平台内置策略库（authz.md §3.1 / design-decisions §25.3）：
// 普通模块接入 L2 从「手写 Resource 实现」降为「一行声明」；工单类复杂策略
// （三轴 + 委托轴）保持手写，两条路并存、永不合流（策略的家 = 数据属主的家）。
//
// 一行接入（模块 wire/启动处即全部权限代码）：
//
//	reg.Register(resource.Builtin("task", resource.OrgMember("jobs")))
//	reg.Register(resource.Builtin("profile", resource.OwnerOnly("user_profiles")))
//	reg.Register(resource.Builtin("misc", resource.RoleGated()))
//
// 策略逻辑在代码、策略数据在 DB（谁绑什么角色进 DB 管理面，模块用哪个行级
// 策略与模块代码同生命周期——不进 DB/管理面，K8s 内置 ClusterRole 同款取舍）。

type builtinKind int

const (
	kindOrgMember builtinKind = iota + 1 // 组员可见：org_id ∈ 用户组织集合
	kindOwnerOnly                        // 属主可见：created_by = 用户
	kindRoleGated                        // 无行级概念：L1 权限码已挡，IW4 显式豁免
)

// BuiltinPolicy 内置策略。table 为模块资源表名（org-member/owner-only 必填：
// 用于 schema 约定校验与文档；role-gated 无表概念）。
type BuiltinPolicy struct {
	kind  builtinKind
	table string
}

// OrgMember 组员策略：行级 = 资源表 org_id 落在用户有效组织集合内
// （user_orgs 含过期过滤，与会话/委托判定同语义）。schema 约定：表必有 org_id。
// 首个消费者 = M-E 任务提交/回调端点（16 号 §3，authz.md §3.1）。
func OrgMember(table string) BuiltinPolicy {
	return BuiltinPolicy{kind: kindOrgMember, table: table}
}

// OwnerOnly 属主策略：行级 = created_by = 当前用户（个人数据类）。
// schema 约定：表必有 created_by。
func OwnerOnly(table string) BuiltinPolicy {
	return BuiltinPolicy{kind: kindOwnerOnly, table: table}
}

// RoleGated 粗粒度策略：无行级概念——L1 权限码（Casbin）已挡，
// GetFilter 返回 IW4 显式豁免（Unscoped），Authorize 恒 true。
func RoleGated() BuiltinPolicy {
	return BuiltinPolicy{kind: kindRoleGated}
}

// BuiltinResource 由内置策略构成的 Resource 实现。
type BuiltinResource struct {
	code string
	p    BuiltinPolicy
	// Membership org-member 的单条/端点判定依赖（SELECT EXISTS(user_orgs ...)）。
	// 由模块注入（闭包其 repo 的一条查询），保持本包零 DB 驱动依赖；
	// 仅用 GetFilter 的模块可不注入（列表谓词自包含子查询）。
	Membership func(ctx context.Context, userID, orgID int64) (bool, error)
}

// Builtin 一行注册：声明资源 code + 内置策略。
func Builtin(code string, p BuiltinPolicy) *BuiltinResource {
	return &BuiltinResource{code: code, p: p}
}

func (b *BuiltinResource) Code() string { return b.code }

func (b *BuiltinResource) Name() string {
	switch b.p.kind {
	case kindOrgMember:
		return fmt.Sprintf("%s (builtin:org-member)", b.code)
	case kindOwnerOnly:
		return fmt.Sprintf("%s (builtin:owner-only)", b.code)
	default:
		return fmt.Sprintf("%s (builtin:role-gated)", b.code)
	}
}

// Actions 内置策略的动作集为通用 CRUD——role-gated 语义下 L1 才是真闸门，
// org-member/owner-only 对动作不区分（行级读写同权）。
func (b *BuiltinResource) Actions() []string {
	return []string{"list", "create", "read", "update", "delete"}
}

// GetFilter 列表行级过滤。谓词引用外层查询的关联列（org_id / created_by），
// 由模块 repo 拼接到自己的列表 SQL——本包不感知表名。
func (b *BuiltinResource) GetFilter(_ context.Context, userID int64, _ string) (Filter, error) {
	switch b.p.kind {
	case kindOrgMember:
		return Filter{
			Where: `org_id IN (SELECT org_id FROM user_orgs WHERE user_id = $1 AND (expires_at IS NULL OR expires_at > NOW()))`,
			Args:  []interface{}{userID},
		}, nil
	case kindOwnerOnly:
		return Filter{
			Where: `created_by = $1`,
			Args:  []interface{}{userID},
		}, nil
	default: // kindRoleGated
		// IW4：无行级概念的模块显式豁免（零值 Filter{} 会被 repo 哨兵拒绝）
		return Filter{Unscoped: true}, nil
	}
}

// Authorize 单条/端点判定。
//
//   - role-gated 恒 true（L1 已挡）；
//   - admin/superadmin bypass（与工单 Resource 对齐）；
//   - owner-only：调用方在 Context["created_by"] 传入行属主，纯函数比对；
//   - org-member：调用方在 Context["org_id"] 传入目标组织，经注入的 Membership
//     查询判定——缺注入或缺 org_id 均 fail-closed 报错（不静默放行）。
func (b *BuiltinResource) Authorize(ctx context.Context, req AuthorizeRequest) (bool, error) {
	switch b.p.kind {
	case kindRoleGated:
		return true, nil
	}
	if hasRole(req.Roles, "admin") || hasRole(req.Roles, "superadmin") {
		return true, nil
	}
	switch b.p.kind {
	case kindOwnerOnly:
		owner, _ := req.Context["created_by"].(int64)
		return owner == req.UserID, nil
	default: // kindOrgMember
		orgID, ok := req.Context["org_id"].(int64)
		if !ok || orgID <= 0 {
			return false, fmt.Errorf("builtin org-member: AuthorizeRequest.Context[org_id] required (int64), got %v", req.Context["org_id"])
		}
		if b.Membership == nil {
			return false, fmt.Errorf("builtin org-member: Membership dependency not injected (fail-closed)")
		}
		return b.Membership(ctx, req.UserID, orgID)
	}
}

// SchemaColumns 返回本策略的 schema 约定列（fail-fast：模块启动时自查资源表，
// 缺列即启动报错，不留到运行时 SQL 报错）。role-gated 无约定。
func (b *BuiltinResource) SchemaColumns() []string {
	switch b.p.kind {
	case kindOrgMember:
		return []string{"org_id"}
	case kindOwnerOnly:
		return []string{"created_by"}
	default:
		return nil
	}
}

// RequireSchema 校验资源表满足 schema 约定。exists 由模块注入
// （SELECT EXISTS(information_schema.columns ...) 的一条闭包，同 Membership 的
// 解耦理由）。返回错误 = 表或约定列缺失，模块应在启动路径上 panic/终止。
func (b *BuiltinResource) RequireSchema(exists func(table, column string) bool) error {
	if b.p.table == "" && b.p.kind != kindRoleGated {
		return fmt.Errorf("builtin %s: table name required for %s policy", b.code, b.p.kind)
	}
	for _, col := range b.SchemaColumns() {
		if !exists(b.p.table, col) {
			return fmt.Errorf("builtin %s: table %s missing required column %q (schema contract, authz.md §3.1)",
				b.code, b.p.table, col)
		}
	}
	return nil
}

// hasRole 精确匹配角色码（与 ticket.HasRole 同语义，本包内自持避免反向依赖）。
func hasRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target {
			return true
		}
	}
	return false
}

// kindName 策略名（错误信息用）。
func (k builtinKind) String() string {
	switch k {
	case kindOrgMember:
		return "org-member"
	case kindOwnerOnly:
		return "owner-only"
	default:
		return "role-gated"
	}
}

// _ 断言 BuiltinResource 实现 Resource 接口（编译期护栏）。
var _ Resource = (*BuiltinResource)(nil)

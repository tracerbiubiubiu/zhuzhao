package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// OrgDelegationService 组织内委托（2c，04-org-delegation SSOT）。
// 三轴不混：全局角色（user_roles/Casbin）/ 组内级别（org_member_role）/ 数据 scope（ticket_scope）。
// L1 Casbin 放行后的细粒度校验（谁能动哪个 org / 哪张工单）在本层完成。
type OrgDelegationService struct {
	db *pgxpool.Pool
}

// NewOrgDelegationService 创建委托服务
func NewOrgDelegationService(db *pgxpool.Pool) *OrgDelegationService {
	return &OrgDelegationService{db: db}
}

// 组内 priority（04 §2.2：数字越小权限越高，与全局 roles.priority 同语义）
const (
	OrgRoleOwnerPriority  = 1
	OrgRoleAdminPriority  = 10
	OrgRoleMemberPriority = 20
)

// EffectiveOrgPriority 组内有效级别（04 §2.2）：
// user ∈ org.owner_user_ids → owner(1)（双轨对齐的另一轨）；否则取 user_orgs.org_member_role。
// 非成员且非 owner → member(20)。
func (s *OrgDelegationService) EffectiveOrgPriority(ctx context.Context, userID, orgID int64) (int, error) {
	// $1/$3 同值（$1 用于数组 ANY、$3 用于等值），规避 pgx 对同一占位符的
	// 多上下文类型推断冲突（42P08）——扩展协议按位置传参，拆占位符即可
	q := `
	SELECT
		CASE
			WHEN NOT (` + repository.OrgDelegationActiveSQL("o") + `) THEN 20
			WHEN $1 = ANY(o.owner_user_ids) THEN 1
			WHEN m.org_member_role = 'owner' THEN 1
			WHEN m.org_member_role = 'admin' THEN 10
			ELSE 20
		END
	FROM organizations o
	LEFT JOIN user_orgs m ON m.org_id = o.id AND m.user_id = $3
		AND (m.expires_at IS NULL OR m.expires_at > NOW())
	WHERE o.id = $2`
	var priority int
	if err := repository.IAMDB(ctx, s.db).QueryRow(ctx, q, userID, orgID, userID).Scan(&priority); err != nil {
		return OrgRoleMemberPriority, fmt.Errorf("effective org priority: %w", err)
	}
	return priority, nil
}

// IsOrgAdminOrOwner 用户是否为指定 org 的 admin/owner（含 owner_user_ids 双轨）。
// 工单 canOperate 的 D7/D8 判定入口（04 §4.2 第二段 SQL 语义）。
func (s *OrgDelegationService) IsOrgAdminOrOwner(ctx context.Context, userID, orgID int64) (bool, error) {
	q := `
	SELECT
		(` + repository.OrgDelegationActiveSQL("o") + `) AND (
		$1 = ANY(o.owner_user_ids)
		OR EXISTS (
			SELECT 1 FROM user_orgs m
			WHERE m.org_id = o.id AND m.user_id = $3
			  AND m.org_member_role IN ('admin', 'owner')
			  AND (m.expires_at IS NULL OR m.expires_at > NOW())
		)
	) FROM organizations o
	WHERE o.id = $2`
	var ok bool
	if err := repository.IAMDB(ctx, s.db).QueryRow(ctx, q, userID, orgID, userID).Scan(&ok); err != nil {
		return false, fmt.Errorf("is org admin or owner: %w", err)
	}
	return ok, nil
}

// IsAncestorOwner 实体部门 owner 对子树的委托（D9，04 §4.2 ancestor owner SQL）：
// 工单所属 org 的任一祖先（含自身，按parent_id真实ID链）的 owner_user_ids 含 userID。
func (s *OrgDelegationService) IsAncestorOwner(ctx context.Context, userID int64, ticketOrgID int64, _ string) (bool, error) {
	q := `
	SELECT EXISTS (
		SELECT 1
		FROM organizations ancestor
		JOIN organizations o ON ancestor.id IN (` + repository.OrgAncestorIDsSQL("o") + `)
		WHERE o.id = $2
		  AND (` + repository.OrgDelegationActiveSQL("o") + `)
		  AND (` + repository.OrgDelegationActiveSQL("ancestor") + `)
		  AND $1 = ANY(ancestor.owner_user_ids)
	)`
	var ok bool
	if err := repository.IAMDB(ctx, s.db).QueryRow(ctx, q, userID, ticketOrgID).Scan(&ok); err != nil {
		return false, fmt.Errorf("is ancestor owner: %w", err)
	}
	return ok, nil
}

// ensureCanManageMember 组内防提权（04 §3.5 矩阵）：
//   - 全局 org:update（actorIsGlobalAdmin=true）绕过组内校验
//   - owner(1)：可管理任何成员（D4/D5 owner 删 admin）
//   - admin(10)：仅可管理 member(20)；目标 ≤ admin → 50009
//   - member(20)/非成员：70001
//
// targetUserID 为 0 时表示不针对特定成员（如批量/任命自身校验），仅校验调用方档位。
func (s *OrgDelegationService) ensureCanManageMember(ctx context.Context, actorUserID, orgID, targetUserID int64, actorIsGlobalAdmin bool) error {
	if actorIsGlobalAdmin {
		return nil // 平台管理员绕过（04 §3.5）
	}
	callerPriority, err := s.EffectiveOrgPriority(ctx, actorUserID, orgID)
	if err != nil {
		return err
	}
	switch {
	case callerPriority <= OrgRoleOwnerPriority:
		return nil
	case callerPriority <= OrgRoleAdminPriority:
		if targetUserID == 0 {
			return errcode.ErrNotOrgOwner // admin 调用仅 owner 可用的接口（如 SetMemberRole，D3 → 50010）
		}
		targetPriority, err := s.EffectiveOrgPriority(ctx, targetUserID, orgID)
		if err != nil {
			return err
		}
		if targetPriority <= OrgRoleAdminPriority {
			return errcode.ErrCannotManageOrgMember // D5：admin 动 admin/owner
		}
		return nil
	default:
		return errcode.ErrNoPermission // member/非成员无成员管理权
	}
}

// HasOrgManagePermission preserves the ticket creation port: its global
// membership-management exception is org:member, never an arbitrary org button.
func (s *OrgDelegationService) HasOrgManagePermission(ctx context.Context, userID int64) (bool, error) {
	return s.HasOrgActionPermission(ctx, userID, "org:member")
}

// HasOrgActionPermission checks only the named current functional permissions.
// An omitted purpose grants nothing; any org button is never a global wildcard.
func (s *OrgDelegationService) HasOrgActionPermission(ctx context.Context, userID int64, permissions ...string) (bool, error) {
	if len(permissions) == 0 {
		return false, nil
	}
	q := `SELECT EXISTS (
  SELECT 1 FROM menus m JOIN role_menus rm ON rm.menu_id=m.id
  JOIN roles r ON r.id=rm.role_id AND r.deleted_at IS NULL AND r.status=1
  WHERE m.permission=ANY($2::text[]) AND m.deleted_at IS NULL
   AND r.id IN (` + repository.EffectiveRoleCTE() + `SELECT id FROM expanded))`
	var ok bool
	if err := repository.IAMDB(ctx, s.db).QueryRow(ctx, q, userID, permissions).Scan(&ok); err != nil {
		return false, fmt.Errorf("has org manage permission: %w", err)
	}
	return ok, nil
}

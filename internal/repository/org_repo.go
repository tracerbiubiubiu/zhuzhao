package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

const orgSelectColumns = `
	id, code, name, COALESCE(description, '') AS description,
	parent_id, path::text AS path, org_type, status, is_system,
	sort_order, owner_user_ids, created_by, tenant_id, version, deleted_at, created_at, updated_at`

// OrgRepo 组织数据访问
type OrgRepo struct {
	db *pgxpool.Pool
}

func NewOrgRepo(db *pgxpool.Pool) *OrgRepo {
	return &OrgRepo{db: db}
}

func (r *OrgRepo) FindByID(ctx context.Context, id int64) (*model.Organization, error) {
	const q = `SELECT` + orgSelectColumns + `
		FROM organizations WHERE id = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, q, id)
}

func (r *OrgRepo) GetTree(ctx context.Context) ([]*model.Organization, error) {
	const q = `SELECT` + orgSelectColumns + `
		FROM organizations WHERE deleted_at IS NULL
		ORDER BY sort_order ASC, id ASC`
	return r.queryMany(ctx, q)
}

func (r *OrgRepo) GetUserOrgs(ctx context.Context, userID int64) ([]*model.UserOrg, error) {
	rows, err := r.db.Query(ctx, `
		SELECT uo.user_id, uo.org_id, uo.is_primary, uo.joined_at
		FROM user_orgs uo
		INNER JOIN organizations o ON o.id = uo.org_id
		WHERE uo.user_id = $1 AND o.deleted_at IS NULL
		ORDER BY uo.is_primary DESC, uo.org_id ASC`, userID)
	if err != nil {
		return nil, fmt.Errorf("get user orgs: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.UserOrg, error) {
		var uo model.UserOrg
		if err := row.Scan(&uo.UserID, &uo.OrgID, &uo.IsPrimary, &uo.JoinedAt); err != nil {
			return nil, err
		}
		return &uo, nil
	})
}

func (r *OrgRepo) IsMember(ctx context.Context, orgID, userID int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_orgs uo
			INNER JOIN organizations o ON o.id = uo.org_id
			WHERE uo.org_id = $1 AND uo.user_id = $2 AND o.deleted_at IS NULL
		)`, orgID, userID).Scan(&exists)
	return exists, err
}

// AddMember 添加组织成员（幂等）
func (r *OrgRepo) AddMember(ctx context.Context, orgID, userID int64, isPrimary bool) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if isPrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE user_orgs SET is_primary = false WHERE user_id = $1`, userID); err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary)
		VALUES ($1, $2, $3)
		ON CONFLICT (user_id, org_id) DO UPDATE
			-- B3-1：仅 primary=true 时提升；false 不回写（幂等不降级）。
			-- 修复前重复添加未传 is_primary（零值 false）会静默清除已有 primary
			SET is_primary = true
			WHERE EXCLUDED.is_primary`, userID, orgID, isPrimary)
	if err != nil {
		// D2-15：并发双 primary 触发 000008 部分唯一索引 → 409 业务码
		//（B3-3 只接了 SetUserOrgsTx，此处同型遗漏 → 500）
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("add member: %w", err)
	}
	return tx.Commit(ctx)
}

// AddMemberWithRole 添加组织成员并指定组内级别（2c，04 §3.4）。
// 幂等语义与 AddMember 一致（B3-1 primary）；role 由 service 层完成防提权校验。
func (r *OrgRepo) AddMemberWithRole(ctx context.Context, orgID, userID int64, isPrimary bool, role string) error {
	if role == "" {
		role = "member"
	}
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if isPrimary {
		if _, err := tx.Exec(ctx, `
			UPDATE user_orgs SET is_primary = false WHERE user_id = $1`, userID); err != nil {
			return err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary, org_member_role)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, org_id) DO UPDATE
			SET is_primary = true
			WHERE EXCLUDED.is_primary`,
		userID, orgID, isPrimary, role)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("add member with role: %w", err)
	}
	return tx.Commit(ctx)
}

// RemoveMember 移除组织成员（2c 双轨：若目标在 owner_user_ids 中则同步移除——
// 否则残留 effective owner 权限，被移除者仍可 SetOwners/SetMemberRole/删组）
func (r *OrgRepo) RemoveMember(ctx context.Context, orgID, userID int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		DELETE FROM user_orgs WHERE org_id = $1 AND user_id = $2`, orgID, userID)
	if err != nil {
		return fmt.Errorf("remove member: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrNotOrgMember
	}
	// 双轨对齐：从 owner_user_ids 移除（04 §2.2；与 SetOwners 的 owner 同步互逆）
	if _, err := tx.Exec(ctx,
		`UPDATE organizations SET owner_user_ids = array_remove(owner_user_ids, $2), updated_at = NOW() WHERE id = $1`,
		orgID, userID); err != nil {
		return fmt.Errorf("sync owner_user_ids on remove: %w", err)
	}
	return tx.Commit(ctx)
}

// SetUserOrgs 全量覆盖用户组织
func (r *OrgRepo) SetUserOrgs(ctx context.Context, userID int64, orgIDs []int64, primaryOrgID *int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := r.SetUserOrgsTx(ctx, tx, userID, orgIDs, primaryOrgID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// SetUserOrgsTx 在外部事务内全量覆盖用户组织
func (r *OrgRepo) SetUserOrgsTx(ctx context.Context, tx pgx.Tx, userID int64, orgIDs []int64, primaryOrgID *int64) error {
	if _, err := tx.Exec(ctx, `DELETE FROM user_orgs WHERE user_id = $1`, userID); err != nil {
		return err
	}
	// B3-4：入参去重（保序）——修复前重复 org_id 触发 user_orgs 主键冲突 → 500
	seen := make(map[int64]struct{}, len(orgIDs))
	deduped := make([]int64, 0, len(orgIDs))
	for _, orgID := range orgIDs {
		if _, ok := seen[orgID]; ok {
			continue
		}
		seen[orgID] = struct{}{}
		deduped = append(deduped, orgID)
	}
	// P0 同源（2c 双轨）：全量覆盖把用户移出某 org 成员身份后，若其在该 org
	// owner_user_ids 中——同步移除（否则残留 effective owner 权限，仍可
	// SetOwners/SetMemberRole/删组/工单委托）。用户仍是成员的 org 保留 owner。
	// orgIDs 为空（清空全部组织）时 NOT(ANY('{}'))=true → 清理全部 owner 引用。
	if _, err := tx.Exec(ctx, `
		UPDATE organizations SET owner_user_ids = array_remove(owner_user_ids, $1), updated_at = NOW()
		WHERE $1 = ANY(owner_user_ids) AND NOT (id = ANY($2::bigint[]))`,
		userID, deduped); err != nil {
		return fmt.Errorf("sync owner_user_ids on set user orgs: %w", err)
	}
	for _, orgID := range deduped {
		isPrimary := primaryOrgID != nil && *primaryOrgID == orgID
		// D2-16：INSERT...SELECT 过滤软删组织（B4-3 只给 SetRolesTx 做了同型
		// 防御）——裸 INSERT 遇软删 org_id 触发 FK 23503 → 500，且静默丢绑定
		tag, err := tx.Exec(ctx, `
			INSERT INTO user_orgs (user_id, org_id, is_primary)
			SELECT $1, id, $3 FROM organizations
			WHERE id = $2 AND deleted_at IS NULL`, userID, orgID, isPrimary)
		if err != nil {
			// B3-3：并发双 primary 触发部分唯一索引 → 409 业务码（非 500）
			if ec := mapUniqueViolation(err); ec != nil {
				return ec
			}
			return fmt.Errorf("insert user org: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return errcode.ErrOrgNotFound
		}
	}
	return nil
}

func (r *OrgRepo) CountChildren(ctx context.Context, orgID int64) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM organizations
		WHERE parent_id = $1 AND deleted_at IS NULL`, orgID).Scan(&n)
	return n, err
}

func (r *OrgRepo) CountMembers(ctx context.Context, orgID int64) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM user_orgs uo
		INNER JOIN organizations o ON o.id = uo.org_id
		INNER JOIN users u ON u.id = uo.user_id
		WHERE uo.org_id = $1 AND o.deleted_at IS NULL AND u.deleted_at IS NULL`, orgID).Scan(&n)
	return n, err
}

func (r *OrgRepo) Create(ctx context.Context, org *model.Organization) error {
	tenantID := org.TenantID
	if tenantID == 0 {
		tenantID = 1
	}
	status := org.Status
	if status == 0 {
		status = 1
	}
	const q = `
		INSERT INTO organizations (
			code, name, description, parent_id, path, org_type, status, is_system,
			sort_order, created_by, tenant_id
		) VALUES (
			$1, $2, NULLIF($3, ''), $4, $5::ltree, $6, $7, false,
			$8, $9, $10
		)
		RETURNING id, version, created_at, updated_at`
	err := r.db.QueryRow(ctx, q,
		org.Code, org.Name, org.Description, org.ParentID, org.Path,
		org.OrgType, status, org.SortOrder, org.CreatedBy, tenantID,
	).Scan(&org.ID, &org.Version, &org.CreatedAt, &org.UpdatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create org: %w", err)
	}
	org.Status = status
	org.TenantID = tenantID
	return nil
}

func (r *OrgRepo) Update(ctx context.Context, org *model.Organization) error {
	const q = `
		UPDATE organizations SET
			name = $2,
			description = NULLIF($3, ''),
			status = $4,
			sort_order = $5,
			version = version + 1,
			updated_at = NOW()
		WHERE id = $1 AND version = $6 AND deleted_at IS NULL
		RETURNING version, updated_at`
	err := r.db.QueryRow(ctx, q,
		org.ID, org.Name, org.Description, org.Status, org.SortOrder, org.Version,
	).Scan(&org.Version, &org.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrConcurrentModification
		}
		return fmt.Errorf("update org: %w", err)
	}
	return nil
}

func (r *OrgRepo) Delete(ctx context.Context, id int64) error {
	// B4-5：保护检查与软删同事务——原 check-then-act 窗口内加入的成员
	// 不会被拦截，产生「软删组织仍挂成员」的残留行
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 事务内检查（FOR UPDATE 锁行，与写入同快照）
	var children, members int64
	if err := tx.QueryRow(ctx, `
		SELECT
			(SELECT COUNT(*) FROM organizations WHERE parent_id = $1 AND deleted_at IS NULL),
			(SELECT COUNT(*) FROM user_orgs uo
				INNER JOIN organizations o ON o.id = uo.org_id
				INNER JOIN users u ON u.id = uo.user_id
				WHERE uo.org_id = $1 AND o.deleted_at IS NULL AND u.deleted_at IS NULL)`,
		id).Scan(&children, &members); err != nil {
		return fmt.Errorf("check org delete guards: %w", err)
	}
	if children > 0 {
		return errcode.ErrOrgHasChildren
	}
	if members > 0 {
		return errcode.ErrOrgHasMembers
	}

	tag, err := tx.Exec(ctx, `
		UPDATE organizations SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("delete org: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrOrgNotFound
	}
	return tx.Commit(ctx)
}

// Move 移动组织子树（B3-2 事务化重构）。
// 全流程单事务：advisory lock 串行化并发移动 → 事务内重读 path + 环检测 →
// 锁旧子树 → 级联 UPDATE（带行数校验）。
// 修复前环检测/父 path 读取在事务外：并发交叉移动可破坏 ltree 树形不变量，
// 且过期 oldPath 匹配 0 行仍返回成功（静默失效）。
func (r *OrgRepo) Move(ctx context.Context, id int64, newParentID *int64) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 1. 全局串行化并发 move（管理端低频操作，粗粒度锁可接受；
	//    与 AcquireSuperadminGuard 同款 advisory lock 先例）
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('org:move'))`); err != nil {
		return fmt.Errorf("acquire org move lock: %w", err)
	}

	// 2. 事务内重读被移动节点（锁定后快照，消灭 TOCTOU）
	var oldPath, orgCode string
	var isSystem bool
	err = tx.QueryRow(ctx, `
		SELECT path::text, code, is_system FROM organizations
		WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, id).Scan(&oldPath, &orgCode, &isSystem)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrOrgNotFound
		}
		return fmt.Errorf("load moving org: %w", err)
	}
	if isSystem {
		return errcode.ErrOrgIsSystem
	}

	// 3. 新父处理：事务内读父行 + 环检测（ancestor 侧补 deleted_at 过滤）
	newRootPath := orgCode
	var resolvedParentID *int64
	if newParentID != nil {
		if *newParentID == id {
			return errcode.ErrOrgCannotMoveToChild
		}
		var parentPath string
		err := tx.QueryRow(ctx, `
			SELECT id, path::text FROM organizations
			WHERE id = $1 AND deleted_at IS NULL FOR UPDATE`, *newParentID).Scan(newParentID, &parentPath)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return errcode.ErrOrgNotFound
			}
			return fmt.Errorf("load new parent: %w", err)
		}
		// 环检测：新父不得是被移动节点的后代（path 前缀判断）
		var isDescendant bool
		if err := tx.QueryRow(ctx, `
			SELECT $1::ltree <@ $2::ltree`, parentPath, oldPath).Scan(&isDescendant); err != nil {
			return fmt.Errorf("check descendant: %w", err)
		}
		if isDescendant {
			return errcode.ErrOrgCannotMoveToChild
		}
		resolvedParentID = newParentID
		newRootPath = parentPath + "." + orgCode
	}

	// 4. 锁旧子树（谓词补 deleted_at，B3-2 顺带修 R2-ORG-07）
	if _, err := tx.Exec(ctx, `
		SELECT id FROM organizations WHERE path <@ $1::ltree AND deleted_at IS NULL FOR UPDATE`, oldPath); err != nil {
		return fmt.Errorf("lock org subtree: %w", err)
	}

	// 5. 级联 UPDATE + 行数校验（0 行 = 节点被并发移动，冲突而非静默成功）
	tag, err := tx.Exec(ctx, `
		UPDATE organizations
		SET path = CASE
			WHEN nlevel(path) = nlevel(text2ltree($2)) THEN text2ltree($1)
			ELSE text2ltree($1) || subpath(path, nlevel(text2ltree($2)))
		END,
		    parent_id = CASE WHEN id = $3 THEN $4 ELSE parent_id END,
		    updated_at = NOW(),
		    version = version + 1
		WHERE path <@ text2ltree($2) AND deleted_at IS NULL`,
		newRootPath, oldPath, id, resolvedParentID,
	)
	if err != nil {
		return fmt.Errorf("move org subtree: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrConcurrentModification
	}

	// 6. P2-D1：级联更新 tickets.org_path（冗余路径同步）。
	// tickets.org_path 是工单创建时从 organizations.path 快照的冗余列，2b scope=group
	// 的 ltree 可见性过滤依赖它。组织子树移动后必须同步重映射，否则存量工单在
	// 新组织树下「静默失踪」（被 2b 的 ltree 过滤漏掉）。
	// 转换逻辑与 step 5 的 organizations 完全一致：
	//   - 恰为移动节点本身（nlevel 相等）→ 直接替换为 newRootPath
	//   - 为其后代 → newRootPath || subpath(原 oldPath 之后的部分)
	// tickets 无 deleted_at（Delete 走物理 DELETE + CASCADE），不加该谓词；
	// 0 行命中合法（子树下可能无工单），不视为并发冲突。
	if _, err := tx.Exec(ctx, `
		UPDATE tickets
		SET org_path = CASE
			WHEN nlevel(org_path) = nlevel(text2ltree($2)) THEN text2ltree($1)
			ELSE text2ltree($1) || subpath(org_path, nlevel(text2ltree($2)))
		END,
		    updated_at = NOW()
		WHERE org_path <@ text2ltree($2)`,
		newRootPath, oldPath,
	); err != nil {
		return fmt.Errorf("cascade ticket org_path: %w", err)
	}

	return tx.Commit(ctx)
}

func (r *OrgRepo) queryOne(ctx context.Context, q string, args ...any) (*model.Organization, error) {
	row := r.db.QueryRow(ctx, q, args...)
	org, err := scanOrgRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrOrgNotFound
		}
		return nil, err
	}
	return org, nil
}

func (r *OrgRepo) queryMany(ctx context.Context, q string, args ...any) ([]*model.Organization, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, scanOrgCollectableRow)
}

func scanOrgRow(row pgx.Row) (*model.Organization, error) {
	var o model.Organization
	err := row.Scan(
		&o.ID, &o.Code, &o.Name, &o.Description, &o.ParentID, &o.Path,
		&o.OrgType, &o.Status, &o.IsSystem, &o.SortOrder, &o.OwnerUserIDs, &o.CreatedBy,
		&o.TenantID, &o.Version, &o.DeletedAt, &o.CreatedAt, &o.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

func scanOrgCollectableRow(row pgx.CollectableRow) (*model.Organization, error) {
	return scanOrgRow(row)
}

// --- 2c 组织委托（04-org-delegation §3.2/§3.3） ---

// SetOwnersTx 事务内设置负责人（双轨对齐）：
//  1. UPDATE organizations.owner_user_ids；
//  2. 每个新 owner：确保 user_orgs 行存在且 org_member_role='owner'（无行 INSERT，有行 UPDATE）；
//  3. 被移出列表的用户：若其 org_member_role 因 owner 身份（非 owner_user_ids 独立来源不存在——
//     owner 角色仅经 SetOwners 授予），降为 member（保留成员关系）。
func (r *OrgRepo) SetOwnersTx(ctx context.Context, tx pgx.Tx, orgID int64, ownerUserIDs []int64) error {
	if ownerUserIDs == nil {
		ownerUserIDs = []int64{}
	}
	if _, err := tx.Exec(ctx,
		`UPDATE organizations SET owner_user_ids = $2, updated_at = NOW() WHERE id = $1`,
		orgID, ownerUserIDs); err != nil {
		return fmt.Errorf("set owners: %w", err)
	}
	// 双轨对齐：确保每个 owner 有成员行且角色为 owner（04 §2.2）
	for _, uid := range ownerUserIDs {
		if _, err := tx.Exec(ctx, `
			INSERT INTO user_orgs (user_id, org_id, is_primary, org_member_role)
			VALUES ($1, $2, false, 'owner')
			ON CONFLICT (user_id, org_id) DO UPDATE SET org_member_role = 'owner'`,
			uid, orgID); err != nil {
			return fmt.Errorf("ensure owner membership: %w", err)
		}
	}
	// 移出列表者降级为 member（仍保留成员关系；owner 角色仅经 SetOwners 授予）
	if _, err := tx.Exec(ctx, `
		UPDATE user_orgs SET org_member_role = 'member'
		WHERE org_id = $1 AND org_member_role = 'owner'
		  AND user_id <> ALL($2)`,
		orgID, ownerUserIDs); err != nil {
		return fmt.Errorf("demote removed owners: %w", err)
	}
	return nil
}

// SetMemberRoleTx 事务内变更组内角色（调用方校验在 service 层完成）
func (r *OrgRepo) SetMemberRoleTx(ctx context.Context, tx pgx.Tx, orgID, userID int64, role string) error {
	tag, err := tx.Exec(ctx,
		`UPDATE user_orgs SET org_member_role = $3 WHERE org_id = $1 AND user_id = $2`,
		orgID, userID, role)
	if err != nil {
		return fmt.Errorf("set member role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrNotOrgMember // 50007：目标须为成员（04 §3.3.1）
	}
	return nil
}

// RunInTx 供 service 层组合事务（SetOwners 双轨对齐等）
func (r *OrgRepo) RunInTx(ctx context.Context, fn func(tx pgx.Tx) error) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// CountNonOwnerMembers 统计组织的非 owner 成员数（2c D6：owner 派生行不占位）
func (r *OrgRepo) CountNonOwnerMembers(ctx context.Context, orgID int64) (int64, error) {
	var n int64
	if err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE org_id = $1 AND org_member_role <> 'owner'`, orgID).Scan(&n); err != nil {
		return 0, fmt.Errorf("count non-owner members: %w", err)
	}
	return n, nil
}

// ClearOwnerMemberships 清除组织的 owner 派生成员行（2c D6：虚拟组删除前置——
// owner 行由 SetOwners 双轨生成，组消亡即失效，不构成 50005 的成员占位）
func (r *OrgRepo) ClearOwnerMemberships(ctx context.Context, orgID int64) error {
	_, err := r.db.Exec(ctx,
		`DELETE FROM user_orgs WHERE org_id = $1 AND org_member_role = 'owner'`, orgID)
	if err != nil {
		return fmt.Errorf("clear owner memberships: %w", err)
	}
	return nil
}

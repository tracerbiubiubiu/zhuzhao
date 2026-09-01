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

const roleSelectColumns = `
	id, code, name, COALESCE(description, '') AS description,
	status, priority, parent_id, sort_order, is_system, tenant_id, version,
	deleted_at, created_at, updated_at`

// RoleRepo 角色数据访问
type RoleRepo struct {
	db *pgxpool.Pool
}

func NewRoleRepo(db *pgxpool.Pool) *RoleRepo {
	return &RoleRepo{db: db}
}

func (r *RoleRepo) FindByCode(ctx context.Context, code string) (*model.Role, error) {
	const q = `SELECT` + roleSelectColumns + `
		FROM roles WHERE code = $1 AND deleted_at IS NULL LIMIT 1`
	return r.queryOne(ctx, q, code)
}

func (r *RoleRepo) FindByID(ctx context.Context, id int64) (*model.Role, error) {
	const q = `SELECT` + roleSelectColumns + `
		FROM roles WHERE id = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, q, id)
}

func (r *RoleRepo) List(ctx context.Context, hideSuperadmin bool) ([]*model.Role, error) {
	q := `SELECT` + roleSelectColumns + ` FROM roles WHERE deleted_at IS NULL`
	if hideSuperadmin {
		q += ` AND code <> 'superadmin'`
	}
	q += ` ORDER BY priority ASC, sort_order ASC, id ASC`
	return r.queryMany(ctx, q)
}

func (r *RoleRepo) ListMenuIDsByRoleID(ctx context.Context, roleID int64) ([]int64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT menu_id FROM role_menus WHERE role_id = $1 ORDER BY menu_id`, roleID)
	if err != nil {
		return nil, fmt.Errorf("list role menus: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (int64, error) {
		var id int64
		if err := row.Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	})
}

// AssignMenus 全量覆盖角色菜单，并同步 casbin_rule（admin/superadmin 仅更新 role_menus）
func (r *RoleRepo) AssignMenus(ctx context.Context, role *model.Role, menuIDs []int64, apis []*model.MenuAPI) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM role_menus WHERE role_id = $1`, role.ID); err != nil {
		return fmt.Errorf("delete role_menus: %w", err)
	}
	for _, menuID := range menuIDs {
		// B2-5：INSERT...SELECT 带活跃性条件——与 service 预检双保险，
		// 消灭「预检通过后、事务 INSERT 前」菜单被软删的 TOCTOU 残余窗口
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_menus (role_id, menu_id)
			SELECT $1, id FROM menus
			WHERE id = $2 AND deleted_at IS NULL
			ON CONFLICT (role_id, menu_id) DO NOTHING`, role.ID, menuID); err != nil {
			return fmt.Errorf("insert role_menus: %w", err)
		}
	}

	if role.Code == "admin" || role.Code == "superadmin" {
		return tx.Commit(ctx)
	}

	subject := fmt.Sprintf("role::%s", role.Code)
	if _, err := tx.Exec(ctx, `
		DELETE FROM casbin_rule WHERE ptype = 'p' AND v0 = $1`, subject); err != nil {
		return fmt.Errorf("delete casbin rules: %w", err)
	}

	seen := make(map[string]struct{})
	for _, api := range apis {
		key := api.APIPath + "\x00" + api.APIMethod
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		if _, err := tx.Exec(ctx, `
			INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES ('p', $1, $2, $3)
			ON CONFLICT DO NOTHING`, subject, api.APIPath, api.APIMethod); err != nil {
			return fmt.Errorf("insert casbin rule: %w", err)
		}
	}

	return tx.Commit(ctx)
}

func (r *RoleRepo) CountUsersByRoleID(ctx context.Context, roleID int64) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM user_roles WHERE role_id = $1`, roleID).Scan(&n)
	return n, err
}

func (r *RoleRepo) Create(ctx context.Context, role *model.Role) error {
	tenantID := role.TenantID
	if tenantID == 0 {
		tenantID = 1
	}
	// B4-4：status 语义修正——service 层已区分「未传」（默认 1）与「显式 0」
	//（创建即禁用），repo 不再做零值合并（原逻辑使显式禁用角色无法落库）
	status := role.Status
	if status != 0 && status != 1 {
		status = 1
	}
	const q = `
		INSERT INTO roles (
			code, name, description, status, priority, parent_id, sort_order, is_system, tenant_id
		) VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, false, $8)
		RETURNING id, version, created_at, updated_at`
	err := r.db.QueryRow(ctx, q,
		role.Code, role.Name, role.Description, status, role.Priority, role.ParentID, role.SortOrder, tenantID,
	).Scan(&role.ID, &role.Version, &role.CreatedAt, &role.UpdatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create role: %w", err)
	}
	role.Status = status
	role.TenantID = tenantID
	return nil
}

func (r *RoleRepo) Update(ctx context.Context, role *model.Role) error {
	const q = `
		UPDATE roles SET
			name = $2,
			description = NULLIF($3, ''),
			status = $4,
			priority = $5,
			parent_id = $8,
			sort_order = $6,
			version = version + 1,
			updated_at = NOW()
		WHERE id = $1 AND version = $7 AND deleted_at IS NULL
		RETURNING version, updated_at`
	err := r.db.QueryRow(ctx, q,
		role.ID, role.Name, role.Description, role.Status, role.Priority, role.SortOrder, role.Version, role.ParentID,
	).Scan(&role.Version, &role.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrConcurrentModification
		}
		return fmt.Errorf("update role: %w", err)
	}
	return nil
}

func (r *RoleRepo) Delete(ctx context.Context, id int64, roleCode string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err := tx.Exec(ctx, `DELETE FROM role_menus WHERE role_id = $1`, id); err != nil {
		return fmt.Errorf("delete role_menus: %w", err)
	}
	subject := fmt.Sprintf("role::%s", roleCode)
	if _, err := tx.Exec(ctx, `DELETE FROM casbin_rule WHERE ptype = 'p' AND v0 = $1`, subject); err != nil {
		return fmt.Errorf("delete casbin rules: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE roles SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrRoleNotFound
	}
	return tx.Commit(ctx)
}

func (r *RoleRepo) queryOne(ctx context.Context, q string, args ...any) (*model.Role, error) {
	row := r.db.QueryRow(ctx, q, args...)
	role, err := scanRoleRowDirect(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrRoleNotFound
		}
		return nil, err
	}
	return role, nil
}

func (r *RoleRepo) queryMany(ctx context.Context, q string, args ...any) ([]*model.Role, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query roles: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, scanRoleCollectableRowDirect)
}

func scanRoleRowDirect(row pgx.Row) (*model.Role, error) {
	var role model.Role
	err := row.Scan(
		&role.ID, &role.Code, &role.Name, &role.Description, &role.Status, &role.Priority,
		&role.ParentID, &role.SortOrder, &role.IsSystem, &role.TenantID, &role.Version, &role.DeletedAt,
		&role.CreatedAt, &role.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func scanRoleCollectableRowDirect(row pgx.CollectableRow) (*model.Role, error) {
	return scanRoleRowDirect(row)
}

// ListRoleIDsByUserID 查询用户绑定的角色 ID（仅启用中，B1-1：禁用角色的菜单不下发）
func (r *RoleRepo) ListRoleIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	// 2b-org：与 GetEffectiveRoleCodes 同源三源展开（直接 ∪ org_roles ∪ 继承链）——
	// 否则 Casbin L1 放行而 /user/menus 不显示，前后端权限视图脱节
	rows, err := r.db.Query(ctx, `
	WITH RECURSIVE seeds AS (
		SELECT ur.role_id AS id
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id AND r.deleted_at IS NULL AND r.status = 1
		WHERE ur.user_id = $1
		UNION
		SELECT orgr.role_id
		FROM org_roles orgr
		JOIN user_orgs m ON m.org_id = orgr.org_id
			AND (m.expires_at IS NULL OR m.expires_at > NOW())
		JOIN roles r ON r.id = orgr.role_id AND r.deleted_at IS NULL AND r.status = 1
		WHERE m.user_id = $1
	),
	expanded AS (
		SELECT id FROM seeds
		UNION
		SELECT r.parent_id
		FROM expanded e
		JOIN roles r ON r.id = e.id AND r.deleted_at IS NULL AND r.status = 1
		WHERE r.parent_id IS NOT NULL
	)
	SELECT DISTINCT r.id FROM expanded x
	JOIN roles r ON r.id = x.id
	ORDER BY r.id`, userID)
	if err != nil {
		return nil, fmt.Errorf("list role ids: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (int64, error) {
		var id int64
		if err := row.Scan(&id); err != nil {
			return 0, err
		}
		return id, nil
	})
}

// ListRoleCodesByRoleIDs 按角色 ID 集合查询角色 code（B4-4：用户权限码的
// admin/superadmin 通配判断）。D2-46：原名 ListRoleCodesByUserIDs 与入参
// 语义不符（实为 roleIDs，WHERE id = ANY($1)），按名传 userIDs 会查错
func (r *RoleRepo) ListRoleCodesByRoleIDs(ctx context.Context, roleIDs []int64) ([]string, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT code FROM roles WHERE id = ANY($1) AND deleted_at IS NULL`, roleIDs)
	if err != nil {
		return nil, fmt.Errorf("list role codes: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (string, error) {
		var code string
		if err := row.Scan(&code); err != nil {
			return "", err
		}
		return code, nil
	})
}

// ListCasbinPoliciesByRoleCode 查询角色 Casbin 策略（不含 bypass 通配）
func (r *RoleRepo) ListCasbinPoliciesByRoleCode(ctx context.Context, roleCode string) ([][3]string, error) {
	subject := fmt.Sprintf("role::%s", roleCode)
	rows, err := r.db.Query(ctx, `
		SELECT v1, v2 FROM casbin_rule
		WHERE ptype = 'p' AND v0 = $1 AND v1 <> '*'`, subject)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][3]string
	for rows.Next() {
		var obj, act string
		if err := rows.Scan(&obj, &act); err != nil {
			return nil, err
		}
		out = append(out, [3]string{subject, obj, act})
	}
	return out, rows.Err()
}

// GetEffectiveRoleCodes 用户有效功能角色码（BFS 三源展开，rbac-inheritance-and-cascade §4）：
//
//	源 1 直接角色（user_roles）
//	源 2 组织角色（org_roles × user_orgs 直接所属节点——不沿部门 ltree 继承），
//	     临时成员（expires_at 已过）不参与展开
//	源 3 角色继承链（roles.parent_id 沿 child→parent 向上取并集）
//
// 全部角色限 status=1 且未软删。消费方：Casbin 中间件（逐角色码 enforce）、
// 工单 Resource（admin/supervisor 判定）。
func (r *RoleRepo) GetEffectiveRoleCodes(ctx context.Context, userID int64) ([]string, error) {
	const q = `
	WITH RECURSIVE seeds AS (
		SELECT ur.role_id AS id
		FROM user_roles ur
		JOIN roles r ON r.id = ur.role_id AND r.deleted_at IS NULL AND r.status = 1
		WHERE ur.user_id = $1
		UNION
		SELECT orgr.role_id
		FROM org_roles orgr
		JOIN user_orgs m ON m.org_id = orgr.org_id
			AND (m.expires_at IS NULL OR m.expires_at > NOW())
		JOIN roles r ON r.id = orgr.role_id AND r.deleted_at IS NULL AND r.status = 1
		WHERE m.user_id = $1
	),
	expanded AS (
		SELECT id FROM seeds
		UNION
		SELECT parent.id
		FROM expanded e
		JOIN roles r ON r.id = e.id AND r.deleted_at IS NULL AND r.status = 1
		JOIN roles parent ON parent.id = r.parent_id AND parent.deleted_at IS NULL AND parent.status = 1
		WHERE r.parent_id IS NOT NULL
	)
	SELECT DISTINCT r.code
	FROM expanded x
	JOIN roles r ON r.id = x.id AND r.deleted_at IS NULL AND r.status = 1
	ORDER BY r.code`
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, fmt.Errorf("get effective role codes: %w", err)
	}
	defer rows.Close()
	codes, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("collect effective role codes: %w", err)
	}
	return codes, nil
}

// EnsureChildrenPriorityAllowed P1-2：角色 priority 变更的子角色守卫。
// 继承不变量 child.priority ≤ parent.priority：把角色改到比任一子角色更弱的
// 档位（数值更小）会破坏单调性——子角色持有者经 BFS 获得变更后更强的父角色，
// 绕过赋角防线。存在 priority > 新值的子角色 → ErrInvalidParams。
// （反向——新值大于全部子角色——是安全弱化，放行。）
func (r *RoleRepo) EnsureChildrenPriorityAllowed(ctx context.Context, roleID int64, newPriority int) error {
	var cnt int
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM roles
		WHERE parent_id = $1 AND priority > $2 AND deleted_at IS NULL`,
		roleID, newPriority).Scan(&cnt)
	if err != nil {
		return fmt.Errorf("check children priority: %w", err)
	}
	if cnt > 0 {
		return errcode.New(errcode.ErrInvalidParams.Code,
			"存在 priority 高于新值的子角色，该变更将破坏继承单调性（child ≤ parent）")
	}
	return nil
}

// RoleRef 继承边校验用的角色引用（BK-12）
type RoleRef struct {
	Priority int
	ParentID *int64
	IsSystem bool
	Status   int
}

// GetRoleRef 读取角色的 priority/parent_id/is_system/status（未删）
func (r *RoleRepo) GetRoleRef(ctx context.Context, id int64) (*RoleRef, error) {
	var ref RoleRef
	err := r.db.QueryRow(ctx, `
		SELECT priority, parent_id, is_system, status
		FROM roles WHERE id = $1 AND deleted_at IS NULL`, id).Scan(
		&ref.Priority, &ref.ParentID, &ref.IsSystem, &ref.Status)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrRoleNotFound
		}
		return nil, fmt.Errorf("get role ref: %w", err)
	}
	return &ref, nil
}

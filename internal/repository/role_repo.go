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
	status, priority, sort_order, is_system, tenant_id, version,
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
		if _, err := tx.Exec(ctx, `
			INSERT INTO role_menus (role_id, menu_id) VALUES ($1, $2)
			ON CONFLICT (role_id, menu_id) DO NOTHING`, role.ID, menuID); err != nil {
			return fmt.Errorf("insert role_menus: %w", err)
		}
	}

	if role.Code == "admin" || role.Code == "superadmin" {
		return tx.Commit(ctx)
	}

	subject := fmt.Sprintf("role::%s", role.Code)
	if _, err := tx.Exec(ctx, `
		DELETE FROM casbin_rule WHERE p_type = 'p' AND v0 = $1`, subject); err != nil {
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
			INSERT INTO casbin_rule (p_type, v0, v1, v2) VALUES ('p', $1, $2, $3)
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
	return fmt.Errorf("not implemented")
}

func (r *RoleRepo) Update(ctx context.Context, role *model.Role) error {
	return fmt.Errorf("not implemented")
}

func (r *RoleRepo) Delete(ctx context.Context, id int64) error {
	return fmt.Errorf("not implemented")
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
		&role.SortOrder, &role.IsSystem, &role.TenantID, &role.Version, &role.DeletedAt,
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

// ListRoleIDsByUserID 查询用户绑定的角色 ID
func (r *RoleRepo) ListRoleIDsByUserID(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.db.Query(ctx, `
		SELECT r.id FROM roles r
		INNER JOIN user_roles ur ON ur.role_id = r.id
		WHERE ur.user_id = $1 AND r.deleted_at IS NULL`, userID)
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

// ListCasbinPoliciesByRoleCode 查询角色 Casbin 策略（不含 bypass 通配）
func (r *RoleRepo) ListCasbinPoliciesByRoleCode(ctx context.Context, roleCode string) ([][3]string, error) {
	subject := fmt.Sprintf("role::%s", roleCode)
	rows, err := r.db.Query(ctx, `
		SELECT v1, v2 FROM casbin_rule
		WHERE p_type = 'p' AND v0 = $1 AND v1 <> '*'`, subject)
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

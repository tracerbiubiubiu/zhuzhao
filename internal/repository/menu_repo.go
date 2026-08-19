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

const menuSelectColumns = `
	id, parent_id, code, name, menu_type,
	COALESCE(path, '') AS path,
	COALESCE(component, '') AS component,
	COALESCE(icon, '') AS icon,
	COALESCE(permission, '') AS permission,
	sort_order, visible, is_system, version, deleted_at, created_at, updated_at`

// MenuRepo 菜单数据访问
type MenuRepo struct {
	db *pgxpool.Pool
}

func NewMenuRepo(db *pgxpool.Pool) *MenuRepo {
	return &MenuRepo{db: db}
}

func (r *MenuRepo) FindByID(ctx context.Context, id int64) (*model.Menu, error) {
	const q = `SELECT` + menuSelectColumns + ` FROM menus WHERE id = $1 AND deleted_at IS NULL`
	return r.queryOne(ctx, q, id)
}

func (r *MenuRepo) ListAll(ctx context.Context) ([]*model.Menu, error) {
	const q = `SELECT` + menuSelectColumns + `
		FROM menus WHERE deleted_at IS NULL ORDER BY sort_order ASC, id ASC`
	return r.queryMany(ctx, q)
}

// ListByRoleIDs 查询角色关联菜单（含按钮，供权限码合并）
func (r *MenuRepo) ListByRoleIDs(ctx context.Context, roleIDs []int64) ([]*model.Menu, error) {
	if len(roleIDs) == 0 {
		return nil, nil
	}
	const q = `
		SELECT DISTINCT m.id, m.parent_id, m.code, m.name, m.menu_type,
			COALESCE(m.path, ''), COALESCE(m.component, ''), COALESCE(m.icon, ''),
			COALESCE(m.permission, ''), m.sort_order, m.visible, m.is_system,
			m.version, m.deleted_at, m.created_at, m.updated_at
		FROM menus m
		INNER JOIN role_menus rm ON rm.menu_id = m.id
		WHERE rm.role_id = ANY($1) AND m.deleted_at IS NULL
		ORDER BY m.sort_order ASC, m.id ASC`
	return r.queryMany(ctx, q, roleIDs)
}

// ListMenuAPIsByMenuIDs 查询菜单绑定的 API（Casbin 策略生成）。
// F-3 修复：去掉 (menu_type = 3 OR api_method = 'GET') 过滤——该条件丢弃页面菜单
// (menu_type=2) 绑定的写 API，而种子数据把全部 POST 路由绑在页面菜单上（按钮菜单
// 零 menu_apis 行），导致自定义角色勾选页面+全部按钮后仍拿不到任何写权限，
// 与 07-menu.md「角色绑定页面菜单即获得该页全部 menu_apis」的设计矛盾。
func (r *MenuRepo) ListMenuAPIsByMenuIDs(ctx context.Context, menuIDs []int64) ([]*model.MenuAPI, error) {
	if len(menuIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT ma.menu_id, ma.api_path, ma.api_method
		FROM menu_apis ma
		JOIN menus m ON m.id = ma.menu_id
		WHERE ma.menu_id = ANY($1)
		  AND m.deleted_at IS NULL
		ORDER BY ma.menu_id, ma.api_path, ma.api_method`, menuIDs)
	if err != nil {
		return nil, fmt.Errorf("list menu apis: %w", err)
	}
	defer rows.Close()

	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.MenuAPI, error) {
		var api model.MenuAPI
		if err := row.Scan(&api.MenuID, &api.APIPath, &api.APIMethod); err != nil {
			return nil, err
		}
		return &api, nil
	})
}

// ListByIDs 按 ID 批量查询
func (r *MenuRepo) ListByIDs(ctx context.Context, ids []int64) ([]*model.Menu, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const q = `SELECT` + menuSelectColumns + `
		FROM menus WHERE id = ANY($1) AND deleted_at IS NULL
		ORDER BY sort_order ASC, id ASC`
	return r.queryMany(ctx, q, ids)
}

func (r *MenuRepo) CountChildren(ctx context.Context, menuID int64) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx, `
		SELECT COUNT(*) FROM menus
		WHERE parent_id = $1 AND deleted_at IS NULL`, menuID).Scan(&n)
	return n, err
}

func (r *MenuRepo) Create(ctx context.Context, menu *model.Menu) error {
	visible := menu.Visible
	const q = `
		INSERT INTO menus (
			parent_id, code, name, menu_type, path, component, icon, permission,
			sort_order, visible, is_system, version
		) VALUES (
			$1, $2, $3, $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), NULLIF($8, ''),
			$9, $10, false, 1
		)
		RETURNING id, version, created_at, updated_at`
	err := r.db.QueryRow(ctx, q,
		menu.ParentID, menu.Code, menu.Name, menu.MenuType,
		menu.Path, menu.Component, menu.Icon, menu.Permission,
		menu.SortOrder, visible,
	).Scan(&menu.ID, &menu.Version, &menu.CreatedAt, &menu.UpdatedAt)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			return ec
		}
		return fmt.Errorf("create menu: %w", err)
	}
	return nil
}

func (r *MenuRepo) Update(ctx context.Context, menu *model.Menu) error {
	const q = `
		UPDATE menus SET
			name = $2,
			path = NULLIF($3, ''),
			component = NULLIF($4, ''),
			icon = NULLIF($5, ''),
			permission = NULLIF($6, ''),
			sort_order = $7,
			visible = $8,
			version = version + 1,
			updated_at = NOW()
		WHERE id = $1 AND version = $9 AND deleted_at IS NULL
		RETURNING version, updated_at`
	err := r.db.QueryRow(ctx, q,
		menu.ID, menu.Name, menu.Path, menu.Component, menu.Icon, menu.Permission,
		menu.SortOrder, menu.Visible, menu.Version,
	).Scan(&menu.Version, &menu.UpdatedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrConcurrentModification
		}
		return fmt.Errorf("update menu: %w", err)
	}
	return nil
}

func (r *MenuRepo) Delete(ctx context.Context, id int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE menus SET deleted_at = NOW(), updated_at = NOW()
		WHERE id = $1 AND deleted_at IS NULL`, id)
	if err != nil {
		return fmt.Errorf("delete menu: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrMenuNotFound
	}
	return nil
}

func (r *MenuRepo) queryOne(ctx context.Context, q string, args ...any) (*model.Menu, error) {
	row := r.db.QueryRow(ctx, q, args...)
	menu, err := scanMenuRow(row)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrMenuNotFound
		}
		return nil, err
	}
	return menu, nil
}

func (r *MenuRepo) queryMany(ctx context.Context, q string, args ...any) ([]*model.Menu, error) {
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("query menus: %w", err)
	}
	defer rows.Close()
	return pgx.CollectRows(rows, scanMenuCollectableRow)
}

func scanMenuRow(row pgx.Row) (*model.Menu, error) {
	var m model.Menu
	err := row.Scan(
		&m.ID, &m.ParentID, &m.Code, &m.Name, &m.MenuType,
		&m.Path, &m.Component, &m.Icon, &m.Permission,
		&m.SortOrder, &m.Visible, &m.IsSystem, &m.Version, &m.DeletedAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func scanMenuCollectableRow(row pgx.CollectableRow) (*model.Menu, error) {
	var m model.Menu
	err := row.Scan(
		&m.ID, &m.ParentID, &m.Code, &m.Name, &m.MenuType,
		&m.Path, &m.Component, &m.Icon, &m.Permission,
		&m.SortOrder, &m.Visible, &m.IsSystem, &m.Version, &m.DeletedAt,
		&m.CreatedAt, &m.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

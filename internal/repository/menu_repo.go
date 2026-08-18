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

// ListMenuAPIsByMenuIDs 查询菜单绑定的 API（Casbin 策略生成）
func (r *MenuRepo) ListMenuAPIsByMenuIDs(ctx context.Context, menuIDs []int64) ([]*model.MenuAPI, error) {
	if len(menuIDs) == 0 {
		return nil, nil
	}
	rows, err := r.db.Query(ctx, `
		SELECT ma.menu_id, ma.api_path, ma.api_method
		FROM menu_apis ma
		JOIN menus m ON m.id = ma.menu_id
		WHERE ma.menu_id = ANY($1)
		  AND (m.menu_type = 3 OR ma.api_method = 'GET')
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

func (r *MenuRepo) Create(ctx context.Context, menu *model.Menu) error {
	return fmt.Errorf("not implemented")
}

func (r *MenuRepo) Update(ctx context.Context, menu *model.Menu) error {
	return fmt.Errorf("not implemented")
}

func (r *MenuRepo) Delete(ctx context.Context, id int64) error {
	return fmt.Errorf("not implemented")
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

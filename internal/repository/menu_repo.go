package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// MenuRepo 菜单数据访问
type MenuRepo struct {
	db *pgxpool.Pool
}

func NewMenuRepo(db *pgxpool.Pool) *MenuRepo {
	return &MenuRepo{db: db}
}

func (r *MenuRepo) FindByID(ctx context.Context, id string) (*model.Menu, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *MenuRepo) GetTree(ctx context.Context) ([]*model.Menu, error) {
	return nil, fmt.Errorf("not implemented")
}

// GetByRoleIDs 根据角色 ID 列表查询关联菜单
func (r *MenuRepo) GetByRoleIDs(ctx context.Context, roleIDs []string) ([]*model.Menu, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *MenuRepo) Create(ctx context.Context, menu *model.Menu) error {
	return fmt.Errorf("not implemented")
}

func (r *MenuRepo) Update(ctx context.Context, menu *model.Menu) error {
	return fmt.Errorf("not implemented")
}

func (r *MenuRepo) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

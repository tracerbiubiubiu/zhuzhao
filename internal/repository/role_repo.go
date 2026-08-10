package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// RoleRepo 角色数据访问
type RoleRepo struct {
	db *pgxpool.Pool
}

func NewRoleRepo(db *pgxpool.Pool) *RoleRepo {
	return &RoleRepo{db: db}
}

func (r *RoleRepo) FindByCode(ctx context.Context, code string) (*model.Role, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *RoleRepo) FindByID(ctx context.Context, id string) (*model.Role, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *RoleRepo) List(ctx context.Context) ([]*model.Role, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *RoleRepo) Create(ctx context.Context, role *model.Role) error {
	return fmt.Errorf("not implemented")
}

func (r *RoleRepo) Update(ctx context.Context, role *model.Role) error {
	return fmt.Errorf("not implemented")
}

func (r *RoleRepo) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

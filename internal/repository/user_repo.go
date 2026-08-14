package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// UserRepo 用户数据访问
type UserRepo struct {
	db *pgxpool.Pool
}

// NewUserRepo 创建 UserRepo
func NewUserRepo(db *pgxpool.Pool) *UserRepo {
	return &UserRepo{db: db}
}

// FindByUsername 管理端/列表：username 精确或模糊查询（非登录）
func (r *UserRepo) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	return nil, fmt.Errorf("not implemented")
}

// FindByEmployeeNo 登录用：工号精确匹配（有值则全局唯一）；0 条视为凭证无效
func (r *UserRepo) FindByEmployeeNo(ctx context.Context, employeeNo string) (*model.User, error) {
	return nil, fmt.Errorf("not implemented")
}

// FindByID 根据 ID 查询用户
func (r *UserRepo) FindByID(ctx context.Context, id int64) (*model.User, error) {
	return nil, fmt.Errorf("not implemented")
}

// Create 创建用户
func (r *UserRepo) Create(ctx context.Context, user *model.User) error {
	return fmt.Errorf("not implemented")
}

// List 分页查询用户列表
func (r *UserRepo) List(ctx context.Context, page, pageSize int) ([]*model.User, int64, error) {
	return nil, 0, fmt.Errorf("not implemented")
}

// Update 更新用户
func (r *UserRepo) Update(ctx context.Context, user *model.User) error {
	return fmt.Errorf("not implemented")
}

// SoftDelete 软删除用户
func (r *UserRepo) SoftDelete(ctx context.Context, id int64) error {
	return fmt.Errorf("not implemented")
}

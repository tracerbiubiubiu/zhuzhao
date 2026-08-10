package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// OrgRepo 组织数据访问
type OrgRepo struct {
	db *pgxpool.Pool
}

func NewOrgRepo(db *pgxpool.Pool) *OrgRepo {
	return &OrgRepo{db: db}
}

func (r *OrgRepo) FindByID(ctx context.Context, id string) (*model.Organization, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *OrgRepo) GetTree(ctx context.Context) ([]*model.Organization, error) {
	return nil, fmt.Errorf("not implemented")
}

func (r *OrgRepo) Create(ctx context.Context, org *model.Organization) error {
	return fmt.Errorf("not implemented")
}

func (r *OrgRepo) Update(ctx context.Context, org *model.Organization) error {
	return fmt.Errorf("not implemented")
}

func (r *OrgRepo) Delete(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

// GetUserOrgs 获取用户所属组织列表
func (r *OrgRepo) GetUserOrgs(ctx context.Context, userID string) ([]*model.UserOrg, error) {
	return nil, fmt.Errorf("not implemented")
}

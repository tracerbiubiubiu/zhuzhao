package service

import (
	"context"
	"fmt"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// RBACService 角色权限管理服务
type RBACService struct {
	roleRepo *repository.RoleRepo
	userRepo *repository.UserRepo
}

func NewRBACService(roleRepo *repository.RoleRepo, userRepo *repository.UserRepo) *RBACService {
	return &RBACService{roleRepo: roleRepo, userRepo: userRepo}
}

func (s *RBACService) ListRoles(ctx context.Context) (interface{}, error) {
	return nil, fmt.Errorf("not implemented")
}

func (s *RBACService) CreateRole(ctx context.Context, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *RBACService) UpdateRole(ctx context.Context, id string, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *RBACService) DeleteRole(ctx context.Context, id string) error {
	return fmt.Errorf("not implemented")
}

// GetRoleCodesByUserID 供 Casbin 中间件查询用户直接角色（Phase 1）
func (s *RBACService) GetRoleCodesByUserID(userID int64) ([]string, error) {
	return s.userRepo.GetRoleCodes(context.Background(), userID)
}

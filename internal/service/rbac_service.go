package service

import (
	"context"
	"fmt"

	"github.com/casbin/casbin/v2"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// RBACService 角色权限管理服务
type RBACService struct {
	roleRepo *repository.RoleRepo
	userRepo *repository.UserRepo
	menuRepo *repository.MenuRepo
	enforcer *casbin.SyncedEnforcer
}

func NewRBACService(
	roleRepo *repository.RoleRepo,
	userRepo *repository.UserRepo,
	menuRepo *repository.MenuRepo,
	enforcer *casbin.SyncedEnforcer,
) *RBACService {
	return &RBACService{
		roleRepo: roleRepo,
		userRepo: userRepo,
		menuRepo: menuRepo,
		enforcer: enforcer,
	}
}

func (s *RBACService) ListRoles(ctx context.Context, hideSuperadmin bool) ([]*model.Role, error) {
	return s.roleRepo.List(ctx, hideSuperadmin)
}

func (s *RBACService) CreateRole(ctx context.Context, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *RBACService) UpdateRole(ctx context.Context, id int64, req interface{}) error {
	return fmt.Errorf("not implemented")
}

func (s *RBACService) DeleteRole(ctx context.Context, id int64) error {
	return fmt.Errorf("not implemented")
}

// AssignMenus 全量覆盖角色菜单并同步 Casbin 策略
func (s *RBACService) AssignMenus(ctx context.Context, req *model.AssignMenusRequest, actorUserID int64) error {
	role, err := s.roleRepo.FindByID(ctx, req.RoleID)
	if err != nil {
		return err
	}
	if role.Code == "superadmin" {
		actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
		if err != nil {
			return err
		}
		if !hasRoleCode(actorRoles, "superadmin") {
			return errcode.ErrNoPermission
		}
	}

	var apis []*model.MenuAPI
	if role.Code != "admin" && role.Code != "superadmin" && len(req.MenuIDs) > 0 {
		apis, err = s.menuRepo.ListMenuAPIsByMenuIDs(ctx, req.MenuIDs)
		if err != nil {
			return err
		}
	}

	if err := s.roleRepo.AssignMenus(ctx, role, req.MenuIDs, apis); err != nil {
		return fmt.Errorf("assign menus: %w", err)
	}
	if role.Code != "admin" && role.Code != "superadmin" {
		if err := s.enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("reload casbin policy: %w", err)
		}
	}
	return nil
}

func (s *RBACService) GetRoleMenuIDs(ctx context.Context, roleID int64) ([]int64, error) {
	if _, err := s.roleRepo.FindByID(ctx, roleID); err != nil {
		return nil, err
	}
	return s.roleRepo.ListMenuIDsByRoleID(ctx, roleID)
}

func (s *RBACService) GetRolePermissions(ctx context.Context, roleID int64) ([][3]string, error) {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if role.Code == "admin" || role.Code == "superadmin" {
		return [][3]string{{fmt.Sprintf("role::%s", role.Code), "*", "*"}}, nil
	}
	policies, err := s.roleRepo.ListCasbinPoliciesByRoleCode(ctx, role.Code)
	if err != nil {
		return nil, err
	}
	return policies, nil
}

// GetRoleCodesByUserID 供 Casbin 中间件查询用户直接角色（Phase 1）
func (s *RBACService) GetRoleCodesByUserID(userID int64) ([]string, error) {
	return s.userRepo.GetRoleCodes(context.Background(), userID)
}

func hasRoleCode(roles []*model.Role, code string) bool {
	for _, r := range roles {
		if r.Code == code {
			return true
		}
	}
	return false
}

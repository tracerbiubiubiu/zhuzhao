package service

import (
	"context"
	"fmt"

	"github.com/casbin/casbin/v3"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/validate"
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

func (s *RBACService) GetRole(ctx context.Context, roleID int64) (*model.Role, error) {
	return s.roleRepo.FindByID(ctx, roleID)
}

func (s *RBACService) CreateRole(ctx context.Context, req *model.CreateRoleRequest, actorUserID int64) (*model.Role, error) {
	if !validate.Identifier(req.Code) {
		return nil, errcode.ErrInvalidParams
	}
	// F-2：priority 提权防护——不得创建高于自身权限档位或越过 superadmin 底线的角色
	if err := s.ensureRolePriorityAllowed(ctx, actorUserID, req.Priority); err != nil {
		return nil, err
	}
	status := req.Status
	if status == 0 {
		status = 1
	}
	role := &model.Role{
		Code:        req.Code,
		Name:        req.Name,
		Description: req.Description,
		Priority:    req.Priority,
		SortOrder:   req.SortOrder,
		Status:      status,
	}
	if err := s.roleRepo.Create(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *RBACService) UpdateRole(ctx context.Context, req *model.UpdateRoleRequest, actorUserID int64) (*model.Role, error) {
	role, err := s.roleRepo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if role.IsSystem {
		return nil, errcode.ErrRoleIsSystem
	}
	// F-2：priority 提权防护——不得把角色改为高于自身权限档位或越过 superadmin 底线
	if err := s.ensureRolePriorityAllowed(ctx, actorUserID, req.Priority); err != nil {
		return nil, err
	}
	role.Name = req.Name
	role.Description = req.Description
	role.Priority = req.Priority
	role.SortOrder = req.SortOrder
	role.Status = req.Status
	role.Version = req.Version
	if err := s.roleRepo.Update(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

// ensureRolePriorityAllowed 校验操作者可设置的 priority 上限（F-2）
func (s *RBACService) ensureRolePriorityAllowed(ctx context.Context, actorUserID int64, priority int) error {
	actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
	if err != nil {
		return err
	}
	if !canSetRolePriority(actorRoles, priority) {
		return errcode.ErrCannotAssignHigherRole
	}
	return nil
}

func (s *RBACService) DeleteRole(ctx context.Context, roleID int64) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return errcode.ErrRoleIsSystem
	}
	n, err := s.roleRepo.CountUsersByRoleID(ctx, roleID)
	if err != nil {
		return err
	}
	if n > 0 {
		return errcode.ErrRoleInUse
	}
	if err := s.roleRepo.Delete(ctx, roleID, role.Code); err != nil {
		return err
	}
	if role.Code != "admin" && role.Code != "superadmin" {
		if err := s.enforcer.LoadPolicy(); err != nil {
			return fmt.Errorf("reload casbin policy: %w", err)
		}
	}
	return nil
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
		if !isSuperadmin(actorRoles) {
			return errcode.ErrNoPermission
		}
	}

	var apis []*model.MenuAPI
	menuIDs := []int64(req.MenuIDs)
	if role.Code != "admin" && role.Code != "superadmin" && len(menuIDs) > 0 {
		apis, err = s.menuRepo.ListMenuAPIsByMenuIDs(ctx, menuIDs)
		if err != nil {
			return err
		}
	}

	if err := s.roleRepo.AssignMenus(ctx, role, menuIDs, apis); err != nil {
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

// GetRoleCodesByUserID 供 Casbin 中间件查询用户直接角色（Phase 1）。
// F-7：接收请求 ctx，使取消/超时可传播（此前 context.Background() 会挂满 statement_timeout）
func (s *RBACService) GetRoleCodesByUserID(ctx context.Context, userID int64) ([]string, error) {
	return s.userRepo.GetRoleCodes(ctx, userID)
}

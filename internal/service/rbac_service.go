package service

import (
	"context"
	"fmt"
	"log/slog"
	"time"

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

// ensureRoleVisible 影子超管读路径（B2-6）：非 superadmin 读 superadmin 角色
// 一律 404（与非存在一致，防推断）——List 已过滤，此处覆盖详情/菜单/策略三个读接口
func (s *RBACService) ensureRoleVisible(ctx context.Context, actorUserID int64, role *model.Role) error {
	if role.Code != "superadmin" {
		return nil
	}
	actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
	if err != nil {
		return err
	}
	if !isSuperadmin(actorRoles) {
		return errcode.ErrRoleNotFound
	}
	return nil
}

// GetRole 角色：非 superadmin 读 superadmin 角色 → 404（B2-6）
func (s *RBACService) GetRole(ctx context.Context, roleID, actorUserID int64) (*model.Role, error) {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureRoleVisible(ctx, actorUserID, role); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *RBACService) CreateRole(ctx context.Context, req *model.CreateRoleRequest, actorUserID int64) (*model.Role, error) {
	if !validate.Identifier(req.Code) {
		return nil, errcode.ErrInvalidParams
	}
	// F-2：priority 提权防护——不得创建高于自身权限档位或越过 superadmin 底线的角色
	if err := s.ensureRolePriorityAllowed(ctx, actorUserID, req.Priority); err != nil {
		return nil, err
	}
	// B4-4：Status 指针化——nil 默认启用；显式传 0 创建即禁用
	status := 1
	if req.Status != nil {
		status = *req.Status
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
	// B1-2：目标角色校验——操作者须严格强于目标角色当前档位，
	// 防止低权角色把更强角色降权后接管其用户（与 ensureRolePriorityAllowed 的新值校验互补）
	if err := s.ensureCanManageRole(ctx, actorUserID, role); err != nil {
		return nil, err
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

func (s *RBACService) DeleteRole(ctx context.Context, roleID, actorUserID int64) error {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		return err
	}
	if role.IsSystem {
		return errcode.ErrRoleIsSystem
	}
	// B1-2：目标角色校验——低权角色不得删除更强角色
	if err := s.ensureCanManageRole(ctx, actorUserID, role); err != nil {
		return err
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
		if err := s.reloadPolicy(role.Code); err != nil {
			return err
		}
	}
	return nil
}

// ensureCanManageRole 角色写操作的目标校验（B1-2）：
// 复用用户模块的 canManageTarget 语义——操作者须严格更强（actorP < targetP），
// superadmin 直通。防止持有 role:write 类策略的低权自定义角色破坏更强角色。
func (s *RBACService) ensureCanManageRole(ctx context.Context, actorUserID int64, target *model.Role) error {
	actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
	if err != nil {
		return err
	}
	if !canManageTarget(actorRoles, []*model.Role{target}) {
		return errcode.ErrCannotManageHigher
	}
	return nil
}

// AssignMenus 全量覆盖角色菜单并同步 Casbin 策略
func (s *RBACService) AssignMenus(ctx context.Context, req *model.AssignMenusRequest, actorUserID int64) error {
	role, err := s.roleRepo.FindByID(ctx, req.RoleID)
	if err != nil {
		return err
	}
	// B1-2：目标角色校验——系统角色（superadmin/admin/operator/viewer）仅 superadmin
	// 可改菜单；自定义角色须操作者严格更强。替换原「仅挡 superadmin」的局部特判，
	// 与 UpdateRole/DeleteRole 的目标校验语义对齐。
	actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
	if err != nil {
		return err
	}
	if role.IsSystem && !isSuperadmin(actorRoles) {
		return errcode.ErrRoleIsSystem
	}
	if !canManageTarget(actorRoles, []*model.Role{role}) {
		return errcode.ErrCannotManageHigher
	}

	var apis []*model.MenuAPI
	menuIDs := []int64(req.MenuIDs)
	if len(menuIDs) > 0 {
		// B2-5：菜单存在性/活跃性预检——不存在或已软删的 ID 返回 ErrMenuNotFound
		// （05-role.md 测试用例承诺；此前 FK 错误冒充 500、软删菜单产生脏绑定）
		active, err := s.menuRepo.ListByIDs(ctx, menuIDs)
		if err != nil {
			return err
		}
		if len(active) != len(menuIDs) {
			return errcode.ErrMenuNotFound
		}
		if role.Code != "admin" && role.Code != "superadmin" {
			apis, err = s.menuRepo.ListMenuAPIsByMenuIDs(ctx, menuIDs)
			if err != nil {
				return err
			}
		}
	}

	if err := s.roleRepo.AssignMenus(ctx, role, menuIDs, apis); err != nil {
		return fmt.Errorf("assign menus: %w", err)
	}
	if role.Code != "admin" && role.Code != "superadmin" {
		if err := s.reloadPolicy(role.Code); err != nil {
			return err
		}
	}
	return nil
}

// reloadPolicy 事务提交后刷新 Casbin 内存策略（B3-5）。
// 失败后果：DB 已生效而内存策略陈旧——权限回收场景被撤销的 API 继续放行，
// 直到下一次成功 LoadPolicy。故：Error 日志（含 subject 便于对账）+ 重试 1 次
// + 明确错误码（调用方感知「已提交但策略刷新失败」）。
func (s *RBACService) reloadPolicy(roleCode string) error {
	const retryDelay = 100 * time.Millisecond
	if err := s.enforcer.LoadPolicy(); err != nil {
		slog.Error("casbin policy reload failed, retrying", "role", roleCode, "err", err)
		time.Sleep(retryDelay)
		if err := s.enforcer.LoadPolicy(); err != nil {
			slog.Error("casbin policy reload failed after retry; DB committed but in-memory policy stale",
				"role", roleCode, "err", err)
			return errcode.ErrPolicyReloadFailed
		}
	}
	return nil
}

func (s *RBACService) GetRoleMenuIDs(ctx context.Context, roleID, actorUserID int64) ([]int64, error) {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureRoleVisible(ctx, actorUserID, role); err != nil {
		return nil, err
	}
	return s.roleRepo.ListMenuIDsByRoleID(ctx, roleID)
}

func (s *RBACService) GetRolePermissions(ctx context.Context, roleID, actorUserID int64) ([][3]string, error) {
	role, err := s.roleRepo.FindByID(ctx, roleID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureRoleVisible(ctx, actorUserID, role); err != nil {
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

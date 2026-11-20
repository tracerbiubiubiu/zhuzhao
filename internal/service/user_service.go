package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// UserService 用户管理服务
type UserService struct {
	storageConfig *pkgstorage.ConfigStore
	storageBucket string
	db            *pgxpool.Pool
	userRepo      *repository.UserRepo
	roleRepo      *repository.RoleRepo
	orgService    *OrgService
	rdb           *goredis.Client
	jwtManager    *jwtpkg.Manager
}

func NewUserService(
	db *pgxpool.Pool,
	userRepo *repository.UserRepo,
	roleRepo *repository.RoleRepo,
	orgService *OrgService,
	rdb *goredis.Client,
	jwtManager *jwtpkg.Manager,
) *UserService {
	return &UserService{
		db:         db,
		userRepo:   userRepo,
		roleRepo:   roleRepo,
		orgService: orgService,
		rdb:        rdb,
		jwtManager: jwtManager,
	}
}

// SetStorageConfig is boot-time assembly: share enabled snapshots with storage
// and pin the bucket to the running client, which also captures it at boot.
func (s *UserService) SetStorageConfig(cfg *pkgstorage.ConfigStore) {
	s.storageConfig = cfg
	s.storageBucket = cfg.Load().Bucket
}

func (s *UserService) updateAvatar(ctx context.Context, user *model.User, profile bool) error {
	key, _ := repository.NormalizeAvatar(user.Avatar)
	if s.storageConfig == nil || !s.storageConfig.Load().Enabled || s.storageBucket == "" {
		return errcode.New(errcode.ErrInternal.Code, "对象存储未启用（storage.enabled=false）")
	}
	user.Avatar = key
	return s.userRepo.UpdateWithAvatar(ctx, user, profile, s.storageBucket)
}

func (s *UserService) List(ctx context.Context, q repository.UserListQuery, actorUserID int64) (*model.UserListResponse, error) {
	actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	if !isSuperadmin(actorRoles) {
		q.ExcludeSuperadminUsers = true
	}
	users, total, err := s.userRepo.List(ctx, q)
	if err != nil {
		return nil, err
	}
	// D2-18：回显与 repo normalizePage 实际执行的 clamp 对齐——
	// 原只 clamp 下限，请求 page_size=200 时实查 100 却回显 200（两视图矛盾）
	page, pageSize := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if page > 10000 {
		page = 10000
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return &model.UserListResponse{
		List:     users,
		Total:    total,
		Page:     page,
		PageSize: pageSize,
	}, nil
}

func (s *UserService) GetByID(ctx context.Context, id int64, actorUserID int64) (*model.User, error) {
	user, err := s.userRepo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if err := s.ensureVisible(ctx, actorUserID, user.ID); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) Create(ctx context.Context, req *model.CreateUserRequest, actorUserID int64) (*model.User, error) {
	if !iamCommandActive(ctx, "user.create") {
		return guardedIAMValue(ctx, s.userRepo.WithIAMGuard, "user.create", actorUserID, s.userRepo, func(ctx context.Context) (*model.User, error) { return s.Create(ctx, req, actorUserID) })
	}
	if req.Username == "" || req.Password == "" {
		return nil, errcode.ErrInvalidParams
	}
	// The new user's identity cannot yet own an existing storage object.
	if repository.InternalAvatar(req.Avatar) {
		return nil, errcode.ErrFileNotFound
	}
	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	user := &model.User{
		Username:      req.Username,
		EmployeeNo:    req.EmployeeNo,
		DomainAccount: req.DomainAccount,
		UserDomain:    req.UserDomain,
		Password:      hash,
		RealName:      req.RealName,
		Email:         req.Email,
		Phone:         req.Phone,
		Avatar:        req.Avatar,
	}

	// 事务外做防提权校验（查 actor 角色 + 校验待分配角色 priority）
	actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
	if err != nil {
		return nil, err
	}
	for _, roleID := range req.RoleIDs {
		role, err := s.roleRepo.FindByID(ctx, roleID)
		if err != nil {
			return nil, err
		}
		if !canAssignRole(actorRoles, role) {
			return nil, errcode.ErrCannotAssignHigherRole
		}
	}
	if len(req.OrgIDs) > 0 {
		if err := s.orgService.authorizeNewMemberships(ctx, actorUserID, 0, []int64(req.OrgIDs)); err != nil {
			return nil, err
		}
	}
	// 校验组织是否存在 + primaryOrgID 在列表内
	if req.PrimaryOrgID != nil {
		found := false
		for _, id := range req.OrgIDs {
			if id == *req.PrimaryOrgID {
				found = true
				break
			}
		}
		if !found {
			return nil, errcode.ErrInvalidParams
		}
	}
	for _, orgID := range req.OrgIDs {
		if _, err := s.orgService.orgRepo.FindByID(ctx, orgID); err != nil {
			return nil, err
		}
	}

	// 同一事务内：创建用户 → 分配角色 → 分配组织，任一步失败整体回滚
	tx, err := repository.IAMDB(ctx, s.db).Begin(ctx)
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)

	if err := s.userRepo.CreateTx(ctx, tx, user); err != nil {
		return nil, err
	}
	if len(req.RoleIDs) > 0 {
		if err := s.userRepo.SetRolesTx(ctx, tx, user.ID, []int64(req.RoleIDs)); err != nil {
			return nil, err
		}
	}
	if len(req.OrgIDs) > 0 {
		if err := s.orgService.SetUserOrgsTx(ctx, tx, &model.SetUserOrgsRequest{
			UserID: user.ID, OrgIDs: req.OrgIDs, PrimaryOrgID: req.PrimaryOrgID,
		}); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	return user, nil
}

func (s *UserService) Update(ctx context.Context, req *model.UpdateUserRequest, actorUserID int64) (*model.User, error) {
	if !iamCommandActive(ctx, "user.update") {
		return guardedIAMValue(ctx, s.userRepo.WithIAMGuard, "user.update", actorUserID, s.userRepo, func(ctx context.Context) (*model.User, error) { return s.Update(ctx, req, actorUserID) })
	}
	user, err := s.userRepo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureCanManage(ctx, actorUserID, user.ID); err != nil {
		return nil, err
	}
	identityChanged := (req.EmployeeNo != nil && *req.EmployeeNo != user.EmployeeNo) ||
		(req.DomainAccount != nil && *req.DomainAccount != user.DomainAccount) ||
		(req.UserDomain != nil && *req.UserDomain != user.UserDomain)
	if identityChanged {
		if err := s.ensureTargetProtection(ctx, actorUserID, user.ID); err != nil {
			return nil, err
		}
	}
	// B2-3 patch 语义：仅覆盖显式传入的字段（nil 跳过；空串 = 显式清空）。
	// SQL 不变（合并进 user 对象后仍全量写，未传字段保持 FindByID 原值）
	if req.EmployeeNo != nil {
		user.EmployeeNo = *req.EmployeeNo
	}
	if req.DomainAccount != nil {
		user.DomainAccount = *req.DomainAccount
	}
	if req.UserDomain != nil {
		user.UserDomain = *req.UserDomain
	}
	if req.RealName != nil {
		user.RealName = *req.RealName
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Phone != nil {
		user.Phone = *req.Phone
	}
	if req.Avatar != nil {
		user.Avatar = *req.Avatar
	}
	user.Version = req.Version
	if req.Avatar != nil && repository.InternalAvatar(*req.Avatar) {
		if err := s.updateAvatar(ctx, user, false); err != nil {
			return nil, err
		}
		return user, nil
	}
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) Delete(ctx context.Context, userID, actorUserID int64) error {
	if !iamCommandActive(ctx, "user.delete") {
		return guardedIAMCommand(ctx, s.userRepo.WithIAMGuard, "user.delete", actorUserID, s.userRepo, func(ctx context.Context) error { return s.Delete(ctx, userID, actorUserID) })
	}
	if userID == actorUserID {
		return errcode.ErrForbidden
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.ensureSensitiveCanManage(ctx, actorUserID, userID); err != nil {
		return err
	}
	if user.IsSystem {
		return errcode.ErrUserIsSystem
	}
	// TOCTOU 修复：最后 superadmin 检查与软删除放同一事务，
	// advisory lock 串行化并发删除，杜绝两个请求同时通过 n<=1 检查
	if ok, err := s.userRepo.IsSuperadminUser(ctx, userID); err != nil {
		return err
	} else if ok {
		if err := s.userRepo.RunInTx(ctx, func(tx pgx.Tx) error {
			if err := repository.AcquireSuperadminGuard(ctx, tx); err != nil {
				return err
			}
			n, err := s.userRepo.CountActiveSuperadminUsersTx(ctx, tx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return errcode.ErrCannotRemoveLastSuperadmin
			}
			return s.userRepo.SoftDeleteTx(ctx, tx, userID)
		}); err != nil {
			return err
		}
		return repository.AfterIAMCommit(ctx, func() error { return revokeUserSessionsWithRetry(ctx, s.rdb, userID, s.jwtManager.AccessTTL()) })
	}
	if err := s.userRepo.SoftDelete(ctx, userID); err != nil {
		return err
	}
	return repository.AfterIAMCommit(ctx, func() error { return revokeUserSessionsWithRetry(ctx, s.rdb, userID, s.jwtManager.AccessTTL()) })
}

func (s *UserService) UpdateStatus(ctx context.Context, req *model.UpdateUserStatusRequest, actorUserID int64) error {
	if !iamCommandActive(ctx, "user.status") {
		return guardedIAMCommand(ctx, s.userRepo.WithIAMGuard, "user.status", actorUserID, s.userRepo, func(ctx context.Context) error { return s.UpdateStatus(ctx, req, actorUserID) })
	}
	// B4-3：不能禁用自己（自禁用后需他人恢复，易产生工单；与 Delete 的自我保护对齐）
	if req.Status == 0 && req.UserID == actorUserID {
		return errcode.ErrForbidden
	}
	// B4-3：is_system 种子用户不可禁用（与 Delete 的 ErrUserIsSystem 保护对齐；
	// 种子 admin 被禁用将失去兜底管理入口）
	target, err := s.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return err
	}
	if req.Status == 0 && target.IsSystem {
		return errcode.ErrUserIsSystem
	}
	if err := s.ensureSensitiveCanManage(ctx, actorUserID, req.UserID); err != nil {
		return err
	}
	if req.Status == 0 {
		// TOCTOU 修复：禁用 superadmin 的最后一人检查与状态更新同事务
		if ok, err := s.userRepo.IsSuperadminUser(ctx, req.UserID); err != nil {
			return err
		} else if ok {
			if err := s.userRepo.RunInTx(ctx, func(tx pgx.Tx) error {
				if err := repository.AcquireSuperadminGuard(ctx, tx); err != nil {
					return err
				}
				n, err := s.userRepo.CountActiveSuperadminUsersTx(ctx, tx)
				if err != nil {
					return err
				}
				if n <= 1 {
					return errcode.ErrCannotRemoveLastSuperadmin
				}
				return s.userRepo.UpdateStatusTx(ctx, tx, req.UserID, req.Status)
			}); err != nil {
				return err
			}
			// 与非超管禁用路径一致：禁用成功后吊销全部会话
			// （JWT 中间件不查 DB status，须靠 disabled 键拦截存量 AT）
			return repository.AfterIAMCommit(ctx, func() error { return revokeUserSessionsWithRetry(ctx, s.rdb, req.UserID, s.jwtManager.AccessTTL()) })
		}
	}
	if err := s.userRepo.UpdateStatus(ctx, req.UserID, req.Status); err != nil {
		return err
	}
	if req.Status == 0 {
		return repository.AfterIAMCommit(ctx, func() error { return revokeUserSessionsWithRetry(ctx, s.rdb, req.UserID, s.jwtManager.AccessTTL()) })
	}
	return repository.AfterIAMCommit(ctx, func() error { return clearUserDisabled(ctx, s.rdb, req.UserID) })
}

func (s *UserService) SetRoles(ctx context.Context, req *model.SetUserRolesRequest, actorUserID int64) error {
	if !iamCommandActive(ctx, "user.roles") {
		return guardedIAMCommand(ctx, s.userRepo.WithIAMGuard, "user.roles", actorUserID, s.userRepo, func(ctx context.Context) error { return s.SetRoles(ctx, req, actorUserID) })
	}
	if err := s.ensureSensitiveCanManage(ctx, actorUserID, req.UserID); err != nil {
		return err
	}
	actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
	if err != nil {
		return err
	}
	return s.userRepo.RunInTx(ctx, func(tx pgx.Tx) error {
		// Keep the existing global guard before the target row lock, matching
		// Delete/UpdateStatus ordering. All reads after Begin use this tx, so
		// competing replacements cannot consume the pool and wait on themselves.
		if err := repository.AcquireSuperadminGuard(ctx, tx); err != nil {
			return err
		}
		oldRoles, err := s.userRepo.LockRoleBindingsTx(ctx, tx, req.UserID)
		if err != nil {
			return err
		}
		old := make(map[int64]*model.Role, len(oldRoles))
		wasSuper := false
		for _, role := range oldRoles {
			if role.DeletedAt != nil {
				return errcode.ErrRoleNotFound
			}
			old[role.ID] = role
			wasSuper = wasSuper || (role.Code == superadminCode && role.Status == 1)
		}
		requested := make(map[int64]bool, len(req.RoleIDs))
		willHaveSuper := false
		for _, roleID := range req.RoleIDs {
			if requested[roleID] {
				continue
			}
			requested[roleID] = true
			role, err := s.roleRepo.FindByIDTx(ctx, tx, roleID)
			if err != nil {
				return err
			}
			if role.Status != 1 {
				if _, retained := old[roleID]; retained {
					continue // preserve the fact, without regranting any live authority
				}
				return errcode.New(errcode.ErrInvalidParams.Code, "已禁用的角色不可新增绑定")
			}
			if !canAssignRole(actorRoles, role) {
				return errcode.ErrCannotAssignHigherRole
			}
			willHaveSuper = willHaveSuper || role.Code == superadminCode
		}
		for _, role := range oldRoles {
			if !requested[role.ID] && !canAssignRole(actorRoles, role) {
				return errcode.ErrCannotAssignHigherRole
			}
		}
		if wasSuper && !willHaveSuper {
			n, err := s.userRepo.CountActiveSuperadminUsersTx(ctx, tx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return errcode.ErrCannotRemoveLastSuperadmin
			}
		}
		return s.userRepo.SetRolesTx(ctx, tx, req.UserID, []int64(req.RoleIDs))
	})
}

func (s *UserService) ResetPassword(ctx context.Context, req *model.ResetPasswordRequest, actorUserID int64) error {
	if !iamCommandActive(ctx, "user.reset") {
		return guardedIAMCommand(ctx, s.userRepo.WithIAMGuard, "user.reset", actorUserID, s.userRepo, func(ctx context.Context) error { return s.ResetPassword(ctx, req, actorUserID) })
	}
	if err := s.ensureSensitiveCanManage(ctx, actorUserID, req.UserID); err != nil {
		// 重置密码场景保留 30005 专用文案（04-user.md 契约）
		if errors.Is(err, errcode.ErrCannotManageHigher) {
			return errcode.ErrCannotResetHigher
		}
		return err
	}
	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.userRepo.UpdatePassword(ctx, req.UserID, hash, true); err != nil {
		return err
	}
	return repository.AfterIAMCommit(ctx, func() error { return revokeUserSessionsWithRetry(ctx, s.rdb, req.UserID, s.jwtManager.AccessTTL()) })
}

func (s *UserService) GetProfile(ctx context.Context, userID int64) (*model.User, error) {
	return s.userRepo.FindByID(ctx, userID)
}

func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req *model.UpdateProfileRequest) (*model.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	// B2-3 patch 语义：同 Update，仅覆盖显式传入字段
	if req.RealName != nil {
		user.RealName = *req.RealName
	}
	if req.Email != nil {
		user.Email = *req.Email
	}
	if req.Phone != nil {
		user.Phone = *req.Phone
	}
	if req.Avatar != nil {
		user.Avatar = *req.Avatar
	}
	if req.Avatar != nil && repository.InternalAvatar(*req.Avatar) {
		if err := s.updateAvatar(ctx, user, true); err != nil {
			return nil, err
		}
		return user, nil
	}
	if err := s.userRepo.UpdateProfile(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) SetUserOrgs(ctx context.Context, req *model.SetUserOrgsRequest, actorUserID int64) error {
	if !iamCommandActive(ctx, "user.orgs") {
		return guardedIAMCommand(ctx, s.userRepo.WithIAMGuard, "user.orgs", actorUserID, s.userRepo, func(ctx context.Context) error { return s.SetUserOrgs(ctx, req, actorUserID) })
	}
	if err := s.ensureSensitiveCanManage(ctx, actorUserID, req.UserID); err != nil {
		return err
	}
	if err := s.orgService.authorizeNewMemberships(ctx, actorUserID, req.UserID, []int64(req.OrgIDs)); err != nil {
		return err
	}
	return s.orgService.SetUserOrgs(ctx, req)
}

// GetUserRoleIDs 查询用户已绑角色 ID 集（P2-5 前端角色回显反查——替代「工号精确
// +角色过滤」N+1 推导；可见性同 GetUserOrgs 的 ensureVisible：目标为 superadmin
// 且 actor 非超管 → 404，其余放行——路由级边界在 L1 Casbin，端点挂 system_user 页）
func (s *UserService) GetUserRoleIDs(ctx context.Context, userID, actorUserID int64) ([]int64, error) {
	if err := s.ensureVisible(ctx, actorUserID, userID); err != nil {
		return nil, err
	}
	return s.roleRepo.ListDirectRoleIDsForEdit(ctx, userID)
}

func (s *UserService) GetUserOrgs(ctx context.Context, userID, actorUserID int64) ([]*model.UserOrg, error) {
	if err := s.ensureVisible(ctx, actorUserID, userID); err != nil {
		return nil, err
	}
	return s.orgService.GetUserOrgs(ctx, userID)
}

// ensureCanManage 通用目标校验（B1-2：错误码通用化）：
// 覆盖 Update/Delete/UpdateStatus/SetRoles/SetUserOrgs 等全部写路径；
// ResetPassword 场景在调用处转换为 30005 专用文案。
func (s *UserService) ensureCanManage(ctx context.Context, actorID, targetID int64) error {
	if actorID == targetID {
		return nil
	}
	actorRoles, err := s.userRepo.GetRoles(ctx, actorID)
	if err != nil {
		return err
	}
	targetRoles, err := s.userRepo.GetRoles(ctx, targetID)
	if err != nil {
		return err
	}
	if !canManageTarget(actorRoles, targetRoles) {
		return errcode.ErrCannotManageHigher
	}
	return nil
}

func (s *UserService) ensureSensitiveCanManage(ctx context.Context, actorID, targetID int64) error {
	if err := s.ensureCanManage(ctx, actorID, targetID); err != nil {
		return err
	}
	return s.ensureTargetProtection(ctx, actorID, targetID)
}

// Only the target side consumes the effective projection. Organization and
// inherited business roles never grant the actor additional IAM authority.
// The original self and superadmin exceptions remain independent of this guard.
func (s *UserService) ensureTargetProtection(ctx context.Context, actorID, targetID int64) error {
	if actorID == targetID {
		return nil
	}
	actorRoles, err := s.userRepo.GetRoles(ctx, actorID)
	if err != nil {
		return err
	}
	if isSuperadmin(actorRoles) {
		return nil
	}
	targetRoles, err := s.roleRepo.GetEffectiveRolesForTargetProtection(ctx, targetID)
	if err != nil {
		return err
	}
	if !canManageTarget(actorRoles, targetRoles) {
		return errcode.ErrCannotManageHigher
	}
	return nil
}

func (s *UserService) ensureVisible(ctx context.Context, actorID, targetID int64) error {
	actorRoles, err := s.userRepo.GetRoles(ctx, actorID)
	if err != nil {
		return err
	}
	if isSuperadmin(actorRoles) {
		return nil
	}
	ok, err := s.userRepo.IsSuperadminUser(ctx, targetID)
	if err != nil {
		return err
	}
	if ok {
		return errcode.ErrUserNotFound
	}
	return nil
}

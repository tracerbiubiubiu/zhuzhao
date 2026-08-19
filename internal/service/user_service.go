package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// UserService 用户管理服务
type UserService struct {
	db         *pgxpool.Pool
	userRepo   *repository.UserRepo
	roleRepo   *repository.RoleRepo
	orgService *OrgService
	rdb        *goredis.Client
	jwtManager *jwtpkg.Manager
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
	page, pageSize := q.Page, q.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
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
	if req.Username == "" || req.Password == "" {
		return nil, errcode.ErrInvalidParams
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
	tx, err := s.db.Begin(ctx)
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
	user, err := s.userRepo.FindByID(ctx, req.ID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureCanManage(ctx, actorUserID, user.ID); err != nil {
		return nil, err
	}
	if req.Username != "" {
		user.Username = req.Username
	}
	user.EmployeeNo = req.EmployeeNo
	user.DomainAccount = req.DomainAccount
	user.UserDomain = req.UserDomain
	user.RealName = req.RealName
	user.Email = req.Email
	user.Phone = req.Phone
	user.Avatar = req.Avatar
	user.Version = req.Version
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) Delete(ctx context.Context, userID, actorUserID int64) error {
	if userID == actorUserID {
		return errcode.ErrForbidden
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return err
	}
	if err := s.ensureCanManage(ctx, actorUserID, userID); err != nil {
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
		return revokeUserSessions(ctx, s.rdb, userID, s.jwtManager.AccessTTL())
	}
	if err := s.userRepo.SoftDelete(ctx, userID); err != nil {
		return err
	}
	return revokeUserSessions(ctx, s.rdb, userID, s.jwtManager.AccessTTL())
}

func (s *UserService) UpdateStatus(ctx context.Context, req *model.UpdateUserStatusRequest, actorUserID int64) error {
	if err := s.ensureCanManage(ctx, actorUserID, req.UserID); err != nil {
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
			return revokeUserSessions(ctx, s.rdb, req.UserID, s.jwtManager.AccessTTL())
		}
	}
	if err := s.userRepo.UpdateStatus(ctx, req.UserID, req.Status); err != nil {
		return err
	}
	if req.Status == 0 {
		return revokeUserSessions(ctx, s.rdb, req.UserID, s.jwtManager.AccessTTL())
	}
	return clearUserDisabled(ctx, s.rdb, req.UserID)
}

func (s *UserService) SetRoles(ctx context.Context, req *model.SetUserRolesRequest, actorUserID int64) error {
	if err := s.ensureCanManage(ctx, actorUserID, req.UserID); err != nil {
		return err
	}
	actorRoles, err := s.userRepo.GetRoles(ctx, actorUserID)
	if err != nil {
		return err
	}
	for _, roleID := range req.RoleIDs {
		role, err := s.roleRepo.FindByID(ctx, roleID)
		if err != nil {
			return err
		}
		if !canAssignRole(actorRoles, role) {
			return errcode.ErrCannotAssignHigherRole
		}
	}
	wasSuper, err := s.userRepo.IsSuperadminUser(ctx, req.UserID)
	if err != nil {
		return err
	}
	willHaveSuper := false
	for _, roleID := range req.RoleIDs {
		role, err := s.roleRepo.FindByID(ctx, roleID)
		if err != nil {
			return err
		}
		if role.Code == "superadmin" {
			willHaveSuper = true
			break
		}
	}
	// TOCTOU 修复：摘除最后一个 superadmin 角色的检查与写入同事务
	if wasSuper && !willHaveSuper {
		return s.userRepo.RunInTx(ctx, func(tx pgx.Tx) error {
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
			return s.userRepo.SetRolesTx(ctx, tx, req.UserID, []int64(req.RoleIDs))
		})
	}
	if err := s.userRepo.SetRoles(ctx, req.UserID, []int64(req.RoleIDs)); err != nil {
		return err
	}
	return nil
}

func (s *UserService) ResetPassword(ctx context.Context, req *model.ResetPasswordRequest, actorUserID int64) error {
	if err := s.ensureCanManage(ctx, actorUserID, req.UserID); err != nil {
		return err
	}
	hash, err := crypto.HashPassword(req.Password)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	if err := s.userRepo.UpdatePassword(ctx, req.UserID, hash, true); err != nil {
		return err
	}
	return revokeUserSessions(ctx, s.rdb, req.UserID, s.jwtManager.AccessTTL())
}

func (s *UserService) GetProfile(ctx context.Context, userID int64) (*model.User, error) {
	return s.userRepo.FindByID(ctx, userID)
}

func (s *UserService) UpdateProfile(ctx context.Context, userID int64, req *model.UpdateProfileRequest) (*model.User, error) {
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	user.RealName = req.RealName
	user.Email = req.Email
	user.Phone = req.Phone
	user.Avatar = req.Avatar
	if err := s.userRepo.Update(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *UserService) SetUserOrgs(ctx context.Context, req *model.SetUserOrgsRequest, actorUserID int64) error {
	if err := s.ensureCanManage(ctx, actorUserID, req.UserID); err != nil {
		return err
	}
	return s.orgService.SetUserOrgs(ctx, req)
}

func (s *UserService) GetUserOrgs(ctx context.Context, userID, actorUserID int64) ([]*model.UserOrg, error) {
	if err := s.ensureVisible(ctx, actorUserID, userID); err != nil {
		return nil, err
	}
	return s.orgService.GetUserOrgs(ctx, userID)
}

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
		return errcode.ErrCannotResetHigher
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

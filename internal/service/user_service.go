package service

import (
	"context"
	"fmt"

	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// UserService 用户管理服务
type UserService struct {
	userRepo   *repository.UserRepo
	roleRepo   *repository.RoleRepo
	orgService *OrgService
	rdb        *goredis.Client
	jwtManager *jwtpkg.Manager
}

func NewUserService(
	userRepo *repository.UserRepo,
	roleRepo *repository.RoleRepo,
	orgService *OrgService,
	rdb *goredis.Client,
	jwtManager *jwtpkg.Manager,
) *UserService {
	return &UserService{
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
		Username:   req.Username,
		EmployeeNo: req.EmployeeNo,
		DomainAccount: req.DomainAccount,
		UserDomain: req.UserDomain,
		Password:   hash,
		RealName:   req.RealName,
		Email:      req.Email,
		Phone:      req.Phone,
		Avatar:     req.Avatar,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}
	if len(req.RoleIDs) > 0 {
		if err := s.SetRoles(ctx, &model.SetUserRolesRequest{UserID: user.ID, RoleIDs: req.RoleIDs}, actorUserID); err != nil {
			return nil, err
		}
	}
	if len(req.OrgIDs) > 0 {
		if err := s.orgService.SetUserOrgs(ctx, &model.SetUserOrgsRequest{
			UserID: user.ID, OrgIDs: req.OrgIDs, PrimaryOrgID: req.PrimaryOrgID,
		}); err != nil {
			return nil, err
		}
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
	if ok, err := s.userRepo.IsSuperadminUser(ctx, userID); err != nil {
		return err
	} else if ok {
		n, err := s.userRepo.CountActiveSuperadminUsers(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return errcode.ErrCannotRemoveLastSuperadmin
		}
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
		if ok, err := s.userRepo.IsSuperadminUser(ctx, req.UserID); err != nil {
			return err
		} else if ok {
			n, err := s.userRepo.CountActiveSuperadminUsers(ctx)
			if err != nil {
				return err
			}
			if n <= 1 {
				return errcode.ErrCannotRemoveLastSuperadmin
			}
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
	if wasSuper && !willHaveSuper {
		n, err := s.userRepo.CountActiveSuperadminUsers(ctx)
		if err != nil {
			return err
		}
		if n <= 1 {
			return errcode.ErrCannotRemoveLastSuperadmin
		}
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

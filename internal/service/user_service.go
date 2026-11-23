package service

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
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
	opsRepo       *repository.SessionOperationRepo
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
		opsRepo:    repository.NewSessionOperationRepo(db),
		orgService: orgService,
		rdb:        rdb,
		jwtManager: jwtManager,
	}
}

// sessionDeps C05 统一安全写依赖集（账号状态/删除/重置与自助改密共用）。
func (s *UserService) sessionDeps() SessionSecurityDeps {
	return SessionSecurityDeps{
		pool:      s.db,
		userRepo:  s.userRepo,
		opsRepo:   s.opsRepo,
		rdb:       s.rdb,
		accessTTL: s.jwtManager.AccessTTL(),
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
	// C05：创建同事务写初始安全事实回执与投影意图（epoch=1/projection=0 由列默认
	// 承载；首版目标代际 1，worker/内联投影确认后登录准入放行）
	opsRepo := repository.NewSessionOperationRepo(s.db)
	initialEpoch := int64(1)
	if err := opsRepo.InsertOperationTx(ctx, tx, &repository.SessionOperation{
		OperationID:      fmt.Sprintf("create:%d", user.ID),
		Namespace:        "http",
		Command:          SessionCmdUserCreate,
		TargetUserID:     user.ID,
		ActorUserID:      actorUserID,
		ExpectedEpoch:    initialEpoch,
		InputFingerprint: SessionBindingFingerprint(SessionCmdUserCreate, user.ID, actorUserID),
		Status:           "completed",
		ResultEpoch:      &initialEpoch,
	}); err != nil {
		return nil, err
	}
	if err := opsRepo.InsertIntentTx(ctx, tx, &repository.SessionProjectionIntent{
		OperationID:   fmt.Sprintf("create:%d", user.ID),
		UserID:        user.ID,
		TargetEpoch:   initialEpoch,
		DesiredActive: user.Status == 1,
		Reason:        "user-create-bootstrap",
	}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, fmt.Errorf("commit tx: %w", err)
	}
	// The nested transaction is a savepoint. Only the owning IAM commit can
	// make the bootstrap intent visible to the projection's separate connection.
	_ = repository.AfterIAMCommit(ctx, func() error {
		_ = tryProjectUserSession(ctx, s.sessionDeps(), user.ID, initialEpoch)
		return nil
	})
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

// Delete 软删除用户（C05：删除同事务推进安全代际、写回执与投影意图；
// Redis 撤销失败不再只是日志——原意图可跨进程恢复）。
func (s *UserService) Delete(ctx context.Context, userID, actorUserID int64, operationID string) (*SessionCommandResult, error) {
	if !iamCommandActive(ctx, "user.delete") {
		return guardedIAMValue(ctx, s.userRepo.WithIAMGuard, "user.delete", actorUserID, s.userRepo, func(ctx context.Context) (*SessionCommandResult, error) {
			return s.Delete(ctx, userID, actorUserID, operationID)
		})
	}
	if userID == actorUserID {
		return nil, errcode.ErrForbidden
	}
	user, err := s.userRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, err
	}
	if err := s.ensureSensitiveCanManage(ctx, actorUserID, userID); err != nil {
		return nil, err
	}
	if user.IsSystem {
		return nil, errcode.ErrUserIsSystem
	}
	deps := s.sessionDeps()
	opID := ResolveOperationID(operationID)
	if res, hit, err := lookUpOperation(ctx, deps, SessionCmdUserDelete, opID, actorUserID, userID); err != nil {
		return nil, err
	} else if hit {
		return res, nil // F021：同 K 重投只返回原结果，不再次执行
	}
	isSuper, err := s.userRepo.IsSuperadminUser(ctx, userID)
	if err != nil {
		return nil, err
	}
	epoch, _, _, err := securityWrite(ctx, deps, SessionCmdUserDelete, opID, "http", actorUserID, userID,
		func(snap *repository.UserSecuritySnapshot) error {
			if snap.Deleted {
				return errcode.ErrUserNotFound
			}
			return nil
		},
		func(tx pgx.Tx, snap *repository.UserSecuritySnapshot) (int64, bool, error) {
			if isSuper {
				// TOCTOU 修复：最后 superadmin 检查与软删除同事务（advisory lock 串行化）
				if err := repository.AcquireSuperadminGuard(ctx, tx); err != nil {
					return 0, false, err
				}
				n, err := s.userRepo.CountActiveSuperadminUsersTx(ctx, tx)
				if err != nil {
					return 0, false, err
				}
				if n <= 1 {
					return 0, false, errcode.ErrCannotRemoveLastSuperadmin
				}
			}
			newEpoch, err := s.userRepo.SoftDeleteWithEpochTx(ctx, tx, userID, snap.SessionEpoch)
			return newEpoch, true, err
		},
		func(*repository.UserSecuritySnapshot) bool { return false }, "user-delete")
	if errors.Is(err, errcode.ErrConcurrentModification) {
		// 同 K 并发：唯一保留失败收敛为读原回执（最多执行一次）
		if res, hit, lerr := lookUpOperation(ctx, deps, SessionCmdUserDelete, opID, actorUserID, userID); lerr == nil && hit {
			return res, nil
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if err := projectAfterCommit(ctx, deps, userID, epoch); err != nil {
		return &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: epoch}, err
	}
	return &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: epoch}, nil
}

// UpdateStatus 启用/禁用（C05：状态与代际同事务推进；禁用提交后 Redis 投影
// 失败保持已提交事实，原意图可恢复；重新启用要求前一撤销已投影确认）。
func (s *UserService) UpdateStatus(ctx context.Context, req *model.UpdateUserStatusRequest, actorUserID int64, operationID string) (*SessionCommandResult, error) {
	if !iamCommandActive(ctx, "user.status") {
		return guardedIAMValue(ctx, s.userRepo.WithIAMGuard, "user.status", actorUserID, s.userRepo, func(ctx context.Context) (*SessionCommandResult, error) {
			return s.UpdateStatus(ctx, req, actorUserID, operationID)
		})
	}
	// B4-3：不能禁用自己（自禁用后需他人恢复，易产生工单；与 Delete 的自我保护对齐）
	if req.Status == 0 && req.UserID == actorUserID {
		return nil, errcode.ErrForbidden
	}
	// B4-3：is_system 种子用户不可禁用（与 Delete 的 ErrUserIsSystem 保护对齐；
	// 种子 admin 被禁用将失去兜底管理入口）
	target, err := s.userRepo.FindByID(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if req.Status == 0 && target.IsSystem {
		return nil, errcode.ErrUserIsSystem
	}
	if err := s.ensureSensitiveCanManage(ctx, actorUserID, req.UserID); err != nil {
		return nil, err
	}
	deps := s.sessionDeps()
	opID := ResolveOperationID(operationID)
	command := SessionCmdUserDisable
	if req.Status == 1 {
		command = SessionCmdUserEnable
	}
	if res, hit, err := lookUpOperation(ctx, deps, command, opID, actorUserID, req.UserID); err != nil {
		return nil, err
	} else if hit {
		return res, nil // F021：同 K 重投只返回原结果，不无意义推进代际
	}
	if req.Status == 1 {
		// enable 门（底座 §4.2）：重新启用不自动清除未完成撤销——已切换账号在
		// 前一撤销投影确认前拒绝启用并保持停用（待恢复冲突）
		switched, err := deps.opsRepo.HasAnyIntentForUser(ctx, req.UserID)
		if err != nil {
			return nil, err
		}
		if switched {
			snap, err := s.userRepo.SecuritySnapshot(ctx, req.UserID)
			if err != nil {
				return nil, err
			}
			if snap.ProjectionEpoch < snap.SessionEpoch {
				return nil, errcode.ErrServiceUnavailable
			}
		}
	}
	isSuper, err := s.userRepo.IsSuperadminUser(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	epoch, changed, _, err := securityWrite(ctx, deps, command, opID, "http", actorUserID, req.UserID,
		func(snap *repository.UserSecuritySnapshot) error {
			if snap.Deleted {
				return errcode.ErrUserNotFound
			}
			return nil
		},
		func(tx pgx.Tx, snap *repository.UserSecuritySnapshot) (int64, bool, error) {
			if req.Status == 0 && snap.Status == 0 {
				// 重复相同目标状态不无意义推进
				return snap.SessionEpoch, false, nil
			}
			if req.Status == 0 && isSuper {
				// TOCTOU 修复：禁用 superadmin 的最后一人检查与状态更新同事务
				if err := repository.AcquireSuperadminGuard(ctx, tx); err != nil {
					return 0, false, err
				}
				n, err := s.userRepo.CountActiveSuperadminUsersTx(ctx, tx)
				if err != nil {
					return 0, false, err
				}
				if n <= 1 {
					return 0, false, errcode.ErrCannotRemoveLastSuperadmin
				}
			}
			newEpoch, changed, err := s.userRepo.AdvanceEpochSetStatusTx(ctx, tx, req.UserID, snap.SessionEpoch, req.Status)
			return newEpoch, changed, err
		},
		func(*repository.UserSecuritySnapshot) bool { return req.Status == 1 }, "user-status")
	if errors.Is(err, errcode.ErrConcurrentModification) {
		// 同 K 并发：唯一保留失败收敛为读原回执
		if res, hit, lerr := lookUpOperation(ctx, deps, command, opID, actorUserID, req.UserID); lerr == nil && hit {
			return res, nil
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	if !changed {
		return &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: epoch}, nil
	}
	// 投影按本事务冻结的 desiredActive 安装（与意图同源；更晚的命令以更新代际取代）
	if err := projectAfterCommit(ctx, deps, req.UserID, epoch); err != nil {
		return &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: epoch}, err
	}
	return &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: epoch}, nil
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

// ResetPassword 管理员重置密码（C05：同事务写口令/代际/回执/意图；保持原
// disabled 状态不顺便启用；F021——同 K 携不同口令均不再次执行，只核查原结果；
// 口令不进回执/指纹/日志）。
func (s *UserService) ResetPassword(ctx context.Context, req *model.ResetPasswordRequest, actorUserID int64, operationID string) (*SessionCommandResult, error) {
	if !iamCommandActive(ctx, "user.reset") {
		return guardedIAMValue(ctx, s.userRepo.WithIAMGuard, "user.reset", actorUserID, s.userRepo, func(ctx context.Context) (*SessionCommandResult, error) {
			return s.ResetPassword(ctx, req, actorUserID, operationID)
		})
	}
	if err := s.ensureSensitiveCanManage(ctx, actorUserID, req.UserID); err != nil {
		// 重置密码场景保留 30005 专用文案（04-user.md 契约）
		if errors.Is(err, errcode.ErrCannotManageHigher) {
			return nil, errcode.ErrCannotResetHigher
		}
		return nil, err
	}
	hash, err := crypto.HashPassword(req.Password) // bcrypt 在锁外计算
	if err != nil {
		return nil, fmt.Errorf("hash password: %w", err)
	}
	deps := s.sessionDeps()
	opID := ResolveOperationID(operationID)
	if res, hit, err := lookUpOperation(ctx, deps, SessionCmdUserResetPwd, opID, actorUserID, req.UserID); err != nil {
		return nil, err
	} else if hit {
		return res, nil // 已存在：明确核查状态，不重放 TokenPair、不比较口令
	}
	epoch, _, _, err := securityWrite(ctx, deps, SessionCmdUserResetPwd, opID, "http", actorUserID, req.UserID,
		func(snap *repository.UserSecuritySnapshot) error {
			if snap.Deleted {
				return errcode.ErrUserNotFound
			}
			return nil
		},
		func(tx pgx.Tx, snap *repository.UserSecuritySnapshot) (int64, bool, error) {
			newEpoch, err := s.userRepo.AdvanceEpochResetPasswordTx(ctx, tx, req.UserID, snap.SessionEpoch, hash)
			return newEpoch, true, err
		},
		func(snap *repository.UserSecuritySnapshot) bool { return snap.Status == 1 },
		"user-reset-password")
	if errors.Is(err, errcode.ErrConcurrentModification) {
		if res, hit, lerr := lookUpOperation(ctx, deps, SessionCmdUserResetPwd, opID, actorUserID, req.UserID); lerr == nil && hit {
			return res, nil
		}
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	// 重置保持原账号状态：投影按本事务冻结的 desiredActive 安装（不顺便启用/禁用）
	if err := projectAfterCommit(ctx, deps, req.UserID, epoch); err != nil {
		return &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: epoch}, err
	}
	return &SessionCommandResult{OperationID: opID, Status: "completed", Epoch: epoch}, nil
}

// UserOperationResultView 受权核查视图（不含口令/token/秘密输入）。
type UserOperationResultView struct {
	OperationID       string `json:"operation_id"`
	Command           string `json:"command,omitempty"`
	Status            string `json:"status"`
	TargetUserID      string `json:"target_user_id,omitempty"`
	AccountStatus     *int   `json:"account_status,omitempty"`
	ResultEpoch       int64  `json:"session_epoch,string,omitempty"`
	ProjectionState   string `json:"projection_state,omitempty"`
	ProjectionPending bool   `json:"projection_pending"`
	CreatedAt         string `json:"created_at,omitempty"`
	UpdatedAt         string `json:"updated_at,omitempty"`
}

// GetOperationResult 按原操作号受权核查。自助改密只允许本人查询，管理回执要求
// 当前具备原命令权限和目标管理权。已获查询准入的 HTTP 调用对缺失或不可见的
// 回执统一返回 not_found，调用方按「待核查」处理。
func (s *UserService) GetOperationResult(ctx context.Context, operationID string, actorUserID int64) (*UserOperationResultView, error) {
	var view *UserOperationResultView
	err := repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		var readErr error
		view, readErr = s.readOperationResult(ctx, tx, operationID, actorUserID)
		return readErr
	})
	if err != nil {
		return nil, err
	}
	return view, nil
}

func (s *UserService) readOperationResult(ctx context.Context, tx pgx.Tx, operationID string, actorUserID int64) (*UserOperationResultView, error) {
	internal := trustedIAMInternal(ctx)
	purpose, admitted := middleware.IAMRequestFromContext(ctx)
	var roles []string
	var policies [][]string
	if !internal {
		if !admitted || !purpose.SelfService || purpose.UserID != actorUserID || purpose.Method != "GET" || purpose.Path != "/api/v1/users/status/result" {
			return nil, errcode.ErrNoPermission
		}
		if err := freshC05Credential(ctx, s.userRepo, purpose); err != nil {
			return nil, err
		}
		var err error
		roles, policies, err = currentIAMAuthorization(ctx, s.userRepo, actorUserID)
		if err != nil {
			return nil, err
		}
	}
	notFound := &UserOperationResultView{OperationID: operationID, Status: "not_found"}
	hide := func(err error) (*UserOperationResultView, error) {
		if internal {
			return nil, err
		}
		return notFound, nil
	}
	op, err := s.opsRepo.FindOperationByID(ctx, operationID)
	if errors.Is(err, errcode.ErrNotFound) {
		return notFound, nil
	}
	if err != nil {
		return nil, err
	}
	if op.Namespace != "http" {
		return hide(errcode.ErrNoPermission)
	}
	var command string
	var requestedStatus *int
	switch op.Command {
	case SessionCmdChangePassword:
		if op.ActorUserID != actorUserID || op.TargetUserID != actorUserID {
			return hide(errcode.ErrNoPermission)
		}
	case SessionCmdUserDisable, SessionCmdUserEnable:
		command = "user.status"
		status := 0
		if op.Command == SessionCmdUserEnable {
			status = 1
		}
		requestedStatus = &status
	case SessionCmdUserDelete:
		command = "user.delete"
	case SessionCmdUserResetPwd:
		command = "user.reset"
	default:
		return hide(errcode.ErrNoPermission)
	}
	if command != "" {
		if !internal {
			if err := authorizeIAMRoute(roles, policies, iamPurposes[command], "POST", purpose.Model); err != nil {
				return hide(err)
			}
		}
		if err := s.ensureSensitiveCanManage(ctx, actorUserID, op.TargetUserID); err != nil {
			if errors.Is(err, errcode.ErrCannotManageHigher) {
				return hide(err)
			}
			return nil, err
		}
	}
	view := &UserOperationResultView{
		OperationID:   op.OperationID,
		Command:       op.Command,
		Status:        op.Status,
		TargetUserID:  strconv.FormatInt(op.TargetUserID, 10),
		AccountStatus: requestedStatus,
		CreatedAt:     op.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     op.UpdatedAt.Format(time.RFC3339),
	}
	if op.ResultEpoch != nil {
		view.ResultEpoch = *op.ResultEpoch
		view.ProjectionState, err = s.opsRepo.OperationProjection(ctx, tx, op.TargetUserID, view.ResultEpoch)
		if err != nil {
			return nil, err
		}
		view.ProjectionPending = view.ProjectionState == "pending"
	}
	return view, nil
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

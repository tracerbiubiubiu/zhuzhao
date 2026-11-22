package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// C05 统一安全事务（底座 §4.2）：HTTP 管理命令、自服务改密与受审 CLI 共用。
// 取适用守卫与用户行，核当前状态/hash/代际，然后同事务写状态或口令、epoch+1、
// 回执与投影意图。DB 是安全代际写主，Redis 是投影；投影在 DB 锁外执行，
// 失败保留 DB 安全事实与 pending，由 worker 按原意图恢复。
const (
	SessionCmdUserDisable      = "user.status.disable"
	SessionCmdUserEnable       = "user.status.enable"
	SessionCmdUserDelete       = "user.delete"
	SessionCmdUserResetPwd     = "user.reset_password"
	SessionCmdChangePassword   = "auth.change_password"
	SessionCmdUserCreate       = "user.create"
	SessionCmdCLIResetPassword = "cli.reset_password"
	SessionCmdProjection       = "session.projection"
)

// SessionCommandResult 统一安全写结果（handler 据此回填 X-Operation-* 响应头）。
type SessionCommandResult struct {
	OperationID string
	Status      string // completed | pending（DB 已提交，投影待恢复）
	Epoch       int64
}

// ResolveOperationID 兼容 header X-Operation-ID：调用方提供则沿用（首次提交前
// 已知稳定请求身份，响应丢失后可按 K 核查）；缺省由服务端生成。
func ResolveOperationID(headerValue string) string {
	if len(headerValue) > 0 && len(headerValue) <= 128 {
		controlled := true
		for _, ch := range headerValue {
			if ch < 0x20 || ch == 0x7f {
				controlled = false
				break
			}
		}
		if controlled {
			return headerValue
		}
	}
	return newOperationID()
}

func newOperationID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("op-%d", time.Now().UnixNano())
	}
	return "op-" + hex.EncodeToString(b)
}

// SessionBindingFingerprint 非秘密绑定投影（命令、目标、可信主体）。口令等秘密
// 输入绝不进入指纹——F021：同 K 携 A/B 均不再次执行，也不比较口令内容。
func SessionBindingFingerprint(command string, targetUserID, actorUserID int64) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s|%d|%d", command, targetUserID, actorUserID)))
	return hex.EncodeToString(sum[:])
}

// SessionSecurityDeps 统一安全写的最小依赖集（UserService/AuthService/admincli 共用）。
type SessionSecurityDeps struct {
	pool      *pgxpool.Pool
	userRepo  *repository.UserRepo
	opsRepo   *repository.SessionOperationRepo
	rdb       *goredis.Client
	accessTTL time.Duration
}

// securityWrite 在 IAM 守卫事务内：锁行快照 → verify 重验 → write 推进 →
// 同事务写回执与意图（receipt/intent 同 tx 是恢复必需）。同 K 并发由
// operation_id 唯一保留收敛：后来者收到 ErrConcurrentModification 后读原回执。
func securityWrite(
	ctx context.Context,
	deps SessionSecurityDeps,
	command, operationID, namespace string,
	actorID, targetID int64,
	verify func(snap *repository.UserSecuritySnapshot) error,
	write func(tx pgx.Tx, snap *repository.UserSecuritySnapshot) (newEpoch int64, changed bool, err error),
	desiredActiveFn func(snap *repository.UserSecuritySnapshot) bool,
	reason string,
) (epoch int64, changed bool, desiredActive bool, err error) {
	tx, err := repository.IAMDB(ctx, deps.pool).Begin(ctx)
	if err != nil {
		return 0, false, false, fmt.Errorf("begin security tx: %w", err)
	}
	defer tx.Rollback(ctx)

	snap, err := deps.userRepo.LockSecuritySnapshotTx(ctx, tx, targetID)
	if err != nil {
		return 0, false, false, err
	}
	desiredActive = desiredActiveFn(snap)
	if err := verify(snap); err != nil {
		return 0, false, false, err
	}
	epoch, changed, werr := write(tx, snap)
	if werr != nil {
		return 0, false, false, werr
	}
	resultEpoch := epoch
	if err := deps.opsRepo.InsertOperationTx(ctx, tx, &repository.SessionOperation{
		OperationID:      operationID,
		Namespace:        namespace,
		Command:          command,
		TargetUserID:     targetID,
		ActorUserID:      actorID,
		ExpectedEpoch:    snap.SessionEpoch,
		InputFingerprint: SessionBindingFingerprint(command, targetID, actorID),
		Status:           "completed",
		ResultEpoch:      &resultEpoch,
	}); err != nil {
		return 0, false, false, err
	}
	if changed {
		if err := deps.opsRepo.InsertIntentTx(ctx, tx, &repository.SessionProjectionIntent{
			OperationID:   operationID,
			UserID:        targetID,
			TargetEpoch:   epoch,
			DesiredActive: desiredActive,
			Reason:        reason,
		}); err != nil {
			return 0, false, false, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, false, false, fmt.Errorf("commit security tx: %w", err)
	}
	return epoch, changed, desiredActive, nil
}

// LookUpOperation 已有操作核查（F021 重复操作语义）：同 K 非秘密绑定一致时
// 返回原操作结果且不再执行秘密写；绑定冲突拒绝且零新增效果。
func lookUpOperation(ctx context.Context, deps SessionSecurityDeps, command, operationID string, actorID, targetID int64) (*SessionCommandResult, bool, error) {
	op, err := deps.opsRepo.FindOperationByID(ctx, operationID)
	if errors.Is(err, errcode.ErrNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	if op.Command != command || op.TargetUserID != targetID || op.ActorUserID != actorID {
		return nil, false, errcode.ErrConflict
	}
	result := &SessionCommandResult{OperationID: op.OperationID, Status: op.Status}
	if op.ResultEpoch != nil {
		result.Epoch = *op.ResultEpoch
		if result.Status == "completed" {
			result.Status, err = deps.opsRepo.OperationProjection(ctx, repository.IAMDB(ctx, deps.pool), targetID, result.Epoch)
			if err != nil {
				return nil, false, err
			}
		}
	}
	return result, true, nil
}

// projectAfterCommit 投影在 DB 锁外执行：失败保留 DB 安全事实与 pending
// （不回滚禁用/改密），worker 按原意图恢复。
func projectAfterCommit(ctx context.Context, deps SessionSecurityDeps, userID, epoch int64) error {
	return repository.AfterIAMCommit(ctx, func() error {
		if err := tryProjectUserSession(ctx, deps, userID, epoch); err != nil {
			// DB 已提交而投影失败：对外统一可恢复 503（安全事实保留，原意图可恢复）
			slog.Error("session projection pending; DB committed, worker will retry intent",
				"user_id", userID, "target_epoch", epoch, "tag", "reconcile", "err", err)
			return errcode.ErrServiceUnavailable
		}
		return nil
	})
}

// tryProjectUserSession 单轮内联投影（少量重试吸收瞬时闪断；最终失败落 pending）。
func tryProjectUserSession(ctx context.Context, deps SessionSecurityDeps, userID, epoch int64) error {
	const attempts = 3
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			select {
			case <-time.After(time.Duration(i) * 100 * time.Millisecond):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err := projectWithLease(ctx, deps, userID, epoch); err != nil {
			lastErr = err
			continue
		}
		snap, err := deps.userRepo.SecuritySnapshot(ctx, userID)
		if err != nil {
			lastErr = err
			continue
		}
		if snap.SessionEpoch == epoch && snap.ProjectionEpoch == epoch {
			return nil
		}
		if snap.SessionEpoch != epoch {
			return errcode.ErrConcurrentModification
		}
		lastErr = errors.New("session projection is pending")
	}
	return lastErr
}

// DB session keys are deliberately disjoint from the legacy Pwe projection.
func dbSessionEpochKey(userID int64) string {
	return fmt.Sprintf("user:session_epoch:db1:%d", userID)
}

func dbSessionDisabledKey(userID int64) string {
	return fmt.Sprintf("user:session_disabled:db1:%d", userID)
}

func dbRefreshKey(userID, epoch int64, deviceID string) string {
	return fmt.Sprintf("refresh:db1:%d:%d:%s", userID, epoch, deviceID)
}

// installProjectionEpochLua compares canonical decimal strings, preserving all
// int64 bits (Lua numbers cannot distinguish adjacent epochs above 2^53).
const installProjectionEpochLua = `
local function valid(v)
  return v and string.match(v, '^[1-9][0-9]*$') and
    (#v < 19 or (#v == 19 and v <= '9223372036854775807'))
end
local target = ARGV[1]
local cur = redis.call('GET', KEYS[1])
if not valid(target) or (cur and not valid(cur)) then
  return redis.error_reply('invalid DB session projection epoch')
end
if ARGV[2] ~= '0' and ARGV[2] ~= '1' then
  return redis.error_reply('invalid DB session projection state')
end
if cur and (#target < #cur or (#target == #cur and target < cur)) then return 0 end
redis.call('SET', KEYS[1], target)
if ARGV[2] == '1' then
  redis.call('DEL', KEYS[2])
else
  redis.call('SET', KEYS[2], '1')
end
return 1`

// installRedisSessionProjection only installs the DB projection. RT slots are
// epoch-scoped: a delayed worker must never scan/delete newer login slots.
func installRedisSessionProjection(ctx context.Context, rdb *goredis.Client, userID, epoch int64, active bool) error {
	installed, err := rdb.Eval(ctx, installProjectionEpochLua,
		[]string{dbSessionEpochKey(userID), dbSessionDisabledKey(userID)},
		strconv.FormatInt(epoch, 10), boolToFlag(active)).Int64()
	if err != nil {
		return fmt.Errorf("install projection epoch: %w", err)
	}
	if installed != 1 {
		return fmt.Errorf("session projection superseded")
	}
	return nil
}

func boolToFlag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// NewSessionWorkerDeps 装配出口：app 装配层构造投影恢复 worker 依赖集。
func NewSessionWorkerDeps(pool *pgxpool.Pool, userRepo *repository.UserRepo, rdb *goredis.Client, accessTTL time.Duration) SessionSecurityDeps {
	return SessionSecurityDeps{
		pool:      pool,
		userRepo:  userRepo,
		opsRepo:   repository.NewSessionOperationRepo(pool),
		rdb:       rdb,
		accessTTL: accessTTL,
	}
}

// RunSessionProjectionWorker 投影恢复 worker（跨进程可恢复）：按原 intent 重投，
// 旧代 superseded，缺项按 DB 代际差值以稳定目标+epoch 补齐；失败保留 pending，
// 不改成新 operation、不因普通重试预算耗尽删项。
func RunSessionProjectionWorker(ctx context.Context, deps SessionSecurityDeps, interval time.Duration) {
	// 启动即先扫一轮（bootstrap 意图尽快确认，避免切换就绪门误伤首次登录）
	runProjectionSweep(ctx, deps)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			runProjectionSweep(ctx, deps)
		}
	}
}

func runProjectionSweep(ctx context.Context, deps SessionSecurityDeps) {
	intents, err := deps.opsRepo.PendingIntents(ctx, 50)
	if err != nil {
		slog.Error("projection worker list pending failed", "err", err)
		return
	}
	seen := map[int64]bool{}
	for _, in := range intents {
		snap, err := deps.userRepo.SecuritySnapshot(ctx, in.UserID)
		if err != nil {
			if !errors.Is(err, errcode.ErrUserNotFound) {
				slog.Error("projection worker snapshot failed", "user_id", in.UserID, "err", err)
			}
			continue
		}
		seen[in.UserID] = true
		// worker 仅按当前 DB 状态和代际投影：旧代 intent superseded
		if in.TargetEpoch < snap.SessionEpoch {
			if _, err := deps.opsRepo.SupersedeIntentsBefore(ctx, in.UserID, snap.SessionEpoch); err != nil {
				slog.Error("projection worker supersede failed", "user_id", in.UserID, "err", err)
				continue
			}
		}
		if err := projectWithLease(ctx, deps, in.UserID, snap.SessionEpoch); err != nil {
			// 失败保留 pending（FailIntent 已释放租约），下轮重投原意图
			slog.Error("projection worker round failed", "user_id", in.UserID,
				"target_epoch", snap.SessionEpoch, "err", err)
		}
	}
	ensureMissingIntents(ctx, deps, seen)
}

// projectWithLease 领取（或合成）当前代意图租约后投影并按原 lease/fence CAS 确认。
func projectWithLease(ctx context.Context, deps SessionSecurityDeps, userID, epoch int64) error {
	intent, err := deps.opsRepo.PendingIntentFor(ctx, userID, epoch)
	if err != nil {
		return err
	}
	if intent == nil {
		return nil
	}
	lease := newOperationID()
	fence, ok, err := deps.opsRepo.AcquireIntentLease(ctx, intent.ID, lease, 30*time.Second)
	if err != nil {
		return err
	}
	if !ok {
		return nil // Another executor is processing or has confirmed this intent.
	}
	if err := installRedisSessionProjection(ctx, deps.rdb, userID, epoch, intent.DesiredActive); err != nil {
		_ = deps.opsRepo.FailIntent(ctx, intent.ID, lease, err.Error())
		return err
	}
	confirmed, err := deps.opsRepo.ConfirmIntent(ctx, intent.ID, lease, fence, epoch)
	if err != nil {
		return err
	}
	if !confirmed {
		// 租约被其他执行者接管：不覆盖结果（FailIntent 也因 token 不匹配为 no-op）
		return fmt.Errorf("projection confirm lost lease (fence %d)", fence)
	}
	return nil
}

// ensureSyntheticIntent worker 缺项发现：不依赖一次性内存队列，以稳定目标+epoch
// 恢复唯一意图（合成 operation_id 保证重入不重复）。
func ensureSyntheticIntent(ctx context.Context, deps SessionSecurityDeps, userID, epoch int64) error {
	tx, err := deps.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin synthetic intent tx: %w", err)
	}
	defer tx.Rollback(ctx)
	snapshot, err := deps.userRepo.LockSecuritySnapshotTx(ctx, tx, userID)
	if err != nil {
		return err
	}
	if snapshot.SessionEpoch != epoch {
		return nil
	}
	synth := fmt.Sprintf("project:%d:%d", userID, epoch)
	op, err := deps.opsRepo.OperationForEffect(ctx, tx, userID, epoch)
	if errors.Is(err, errcode.ErrNotFound) {
		op = &repository.SessionOperation{
			OperationID:      synth,
			Namespace:        "worker",
			Command:          SessionCmdProjection,
			TargetUserID:     userID,
			ActorUserID:      userID,
			ExpectedEpoch:    epoch,
			InputFingerprint: SessionBindingFingerprint(SessionCmdProjection, userID, userID),
			Status:           "completed",
			ResultEpoch:      &epoch,
		}
	} else if err != nil {
		return err
	}
	in := &repository.SessionProjectionIntent{
		OperationID:   op.OperationID,
		UserID:        userID,
		TargetEpoch:   epoch,
		DesiredActive: snapshot.Status == 1 && !snapshot.Deleted,
		Reason:        "worker-gap-discovery",
	}
	if err := deps.opsRepo.EnsureIntentTx(ctx, tx, op, in); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// ensureMissingIntents 缺项兜底：投影落后且无 pending 意图的账号补齐当前代意图
// （下一轮 sweep 投影确认）。分批扫描，避免全表压力。
func ensureMissingIntents(ctx context.Context, deps SessionSecurityDeps, seen map[int64]bool) {
	rows, err := deps.pool.Query(ctx, `
		SELECT u.id, u.session_epoch FROM users u
		WHERE u.session_epoch > u.session_projection_epoch
		  AND NOT EXISTS (
		    SELECT 1 FROM session_projection_intents i
		    WHERE i.user_id = u.id AND i.status = 'pending')
		ORDER BY u.id LIMIT 100`)
	if err != nil {
		slog.Error("projection worker gap scan failed", "err", err)
		return
	}
	type gap struct {
		id    int64
		epoch int64
	}
	gaps := []gap{}
	for rows.Next() {
		var g gap
		if err := rows.Scan(&g.id, &g.epoch); err != nil {
			rows.Close()
			return
		}
		gaps = append(gaps, g)
	}
	rows.Close()
	for _, g := range gaps {
		if seen[g.id] {
			continue
		}
		// 补齐当前代意图并当轮投影确认（不留给下一轮——登录就绪门依赖水位）
		if err := ensureSyntheticIntent(ctx, deps, g.id, g.epoch); err != nil {
			slog.Error("projection worker gap intent failed", "user_id", g.id, "err", err)
			continue
		}
		if err := projectWithLease(ctx, deps, g.id, g.epoch); err != nil {
			slog.Error("projection worker gap project failed", "user_id", g.id, "epoch", g.epoch, "err", err)
		}
	}
}

// CheckProjectionReadiness requires both the DB watermark and the separate
// Redis projection to prove the exact issuing epoch; absence is recoverable 503.
func CheckProjectionReadiness(ctx context.Context, deps SessionSecurityDeps, userID int64, snap *repository.UserSecuritySnapshot) error {
	if snap == nil || snap.SessionEpoch <= 0 || snap.ProjectionEpoch != snap.SessionEpoch || deps.rdb == nil {
		return errcode.ErrServiceUnavailable
	}
	pipe := deps.rdb.Pipeline()
	epochCmd := pipe.Get(ctx, dbSessionEpochKey(userID))
	disabledCmd := pipe.Exists(ctx, dbSessionDisabledKey(userID))
	if _, err := pipe.Exec(ctx); err != nil {
		return errcode.ErrServiceUnavailable
	}
	raw, err := epochCmd.Result()
	if err != nil || raw != strconv.FormatInt(snap.SessionEpoch, 10) {
		return errcode.ErrServiceUnavailable
	}
	disabled, err := disabledCmd.Result()
	if err != nil || disabled != 0 {
		return errcode.ErrServiceUnavailable
	}
	return nil
}

// ProjectAfterCommit 导出投影出口（break-glass admincli 复用）：DB 提交后按
// AfterIAMCommit 尝试投影；失败保留 DB 安全事实与 pending。
func ProjectAfterCommit(ctx context.Context, deps SessionSecurityDeps, userID, epoch int64) error {
	return projectAfterCommit(ctx, deps, userID, epoch)
}

// ExecuteCLIPasswordReset break-glass 重置（namespace=cli）：与 HTTP 管理重置
// 同一统一安全事务（锁行重验 → 口令/代际/回执/意图同事务）；口令不进回执。
func ExecuteCLIPasswordReset(ctx context.Context, deps SessionSecurityDeps, userID int64, operationID, passwordHash string) (int64, bool, bool, error) {
	return securityWrite(ctx, deps, SessionCmdCLIResetPassword, operationID, "cli", userID, userID,
		func(snap *repository.UserSecuritySnapshot) error {
			if snap.Deleted {
				return errcode.ErrUserNotFound
			}
			return nil
		},
		func(tx pgx.Tx, snap *repository.UserSecuritySnapshot) (int64, bool, error) {
			newEpoch, err := deps.userRepo.AdvanceEpochResetPasswordTx(ctx, tx, userID, snap.SessionEpoch, passwordHash)
			return newEpoch, true, err
		},
		func(snap *repository.UserSecuritySnapshot) bool { return snap.Status == 1 },
		"cli-reset-password")
}

// ExecuteCLIEnable requires the original reset generation to still be current and confirmed.
func ExecuteCLIEnable(ctx context.Context, deps SessionSecurityDeps, userID, expectedEpoch int64) (int64, error) {
	opID := "cli-enable:" + fmt.Sprintf("%d:%d", userID, expectedEpoch)
	epoch, _, _, err := securityWrite(ctx, deps, SessionCmdUserEnable, opID, "cli", userID, userID,
		func(snap *repository.UserSecuritySnapshot) error {
			if snap.Deleted {
				return errcode.ErrUserNotFound
			}
			if snap.SessionEpoch != expectedEpoch || snap.Status != 0 {
				return errcode.ErrConcurrentModification
			}
			if snap.ProjectionEpoch != expectedEpoch {
				return errcode.ErrServiceUnavailable
			}
			return nil
		},
		func(tx pgx.Tx, snap *repository.UserSecuritySnapshot) (int64, bool, error) {
			return deps.userRepo.AdvanceEpochSetStatusTx(ctx, tx, userID, snap.SessionEpoch, 1)
		},
		func(*repository.UserSecuritySnapshot) bool { return true },
		"cli-enable")
	if err != nil {
		return 0, err
	}
	return epoch, projectAfterCommit(ctx, deps, userID, epoch)
}

// RunProjectionSweepOnce 导出单轮扫描（集成测试与运维诊断用）：
// 与 worker 周期体完全同一路径——原 intent 重投 + 缺项发现补齐。
func RunProjectionSweepOnce(ctx context.Context, deps SessionSecurityDeps) {
	runProjectionSweep(ctx, deps)
}

package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// SessionOperationRepo C05 私有操作回执与会话投影意图存储
// （迁移 000083；底座 §4.2/§4.4：口令与 token 不落正文，operation 只存
// caller namespace、可信主体、命令种类、目标、冻结 expected_epoch 与非秘密
// 绑定指纹）。
type SessionOperationRepo struct {
	db *pgxpool.Pool
}

func NewSessionOperationRepo(db *pgxpool.Pool) *SessionOperationRepo {
	return &SessionOperationRepo{db: db}
}

// SessionOperation 操作回执行（K 幂等：同 K 最多执行一次）。
type SessionOperation struct {
	OperationID      string
	Namespace        string
	Command          string
	TargetUserID     int64
	ActorUserID      int64
	ExpectedEpoch    int64
	InputFingerprint string
	Status           string // pending | completed | rolled_back
	ResultEpoch      *int64
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

// SessionProjectionIntent 待同步记录（DB 提交后 Redis 投影可跨进程恢复）。
type SessionProjectionIntent struct {
	ID             int64
	OperationID    string
	UserID         int64
	TargetEpoch    int64
	DesiredActive  bool
	Reason         string
	LeaseToken     string
	LeaseExpiresAt *time.Time
	Fence          int64
	Attempts       int
	LastError      string
	Status         string // pending | confirmed | superseded
}

const sessionOpColumns = `operation_id, namespace, command, target_user_id, actor_user_id,
	expected_epoch, input_fingerprint, status, result_epoch, created_at, updated_at`

const insertSessionOperation = `
	INSERT INTO user_session_operations
		(operation_id, namespace, command, target_user_id, actor_user_id,
		 expected_epoch, input_fingerprint, status, result_epoch)
	VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`

func scanSessionOperation(row pgx.Row) (*SessionOperation, error) {
	var op SessionOperation
	err := row.Scan(&op.OperationID, &op.Namespace, &op.Command, &op.TargetUserID,
		&op.ActorUserID, &op.ExpectedEpoch, &op.InputFingerprint, &op.Status,
		&op.ResultEpoch, &op.CreatedAt, &op.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan session operation: %w", err)
	}
	return &op, nil
}

// InsertOperationTx 在安全事务内写操作回执。operation_id 唯一约束是同 K
// 并发首次请求「最多执行一次」的数据库保留手段。
func (r *SessionOperationRepo) InsertOperationTx(ctx context.Context, tx pgx.Tx, op *SessionOperation) error {
	_, err := tx.Exec(ctx, insertSessionOperation,
		op.OperationID, op.Namespace, op.Command, op.TargetUserID, op.ActorUserID,
		op.ExpectedEpoch, op.InputFingerprint, op.Status, op.ResultEpoch)
	if err != nil {
		if ec := mapUniqueViolation(err); ec != nil {
			// 同 K 并发：唯一保留失败由调用方收敛为「读原回执」语义
			return errcode.ErrConcurrentModification
		}
		return fmt.Errorf("insert session operation: %w", err)
	}
	return nil
}

// FindOperationByID 查原回执（受权核查用）。
func (r *SessionOperationRepo) FindOperationByID(ctx context.Context, operationID string) (*SessionOperation, error) {
	return scanSessionOperation(IAMDB(ctx, r.db).QueryRow(ctx,
		`SELECT `+sessionOpColumns+` FROM user_session_operations WHERE operation_id = $1`, operationID))
}

// OperationProjection reads the intent for the exact account generation affected
// by an operation, including a no-op receipt that reused an existing generation.
func (r *SessionOperationRepo) OperationProjection(ctx context.Context, tx IAMDBTX, userID, epoch int64) (string, error) {
	var state string
	err := tx.QueryRow(ctx, `SELECT status FROM session_projection_intents WHERE user_id=$1 AND target_epoch=$2`, userID, epoch).Scan(&state)
	if errors.Is(err, pgx.ErrNoRows) {
		return "pending", nil
	}
	if err != nil {
		return "", fmt.Errorf("read original session projection: %w", err)
	}
	if state == "confirmed" {
		return "completed", nil
	}
	return state, nil
}

// PendingIntentFor finds the pending intent for one account generation, independent of the sweep batch.
func (r *SessionOperationRepo) PendingIntentFor(ctx context.Context, userID, epoch int64) (*SessionProjectionIntent, error) {
	var intent SessionProjectionIntent
	err := r.db.QueryRow(ctx, `SELECT id, desired_active FROM session_projection_intents
		WHERE user_id=$1 AND target_epoch=$2 AND status='pending'`, userID, epoch).
		Scan(&intent.ID, &intent.DesiredActive)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read pending intent: %w", err)
	}
	return &intent, nil
}

// OperationForEffect finds the first durable receipt for an account generation.
// Repair can resume its original K instead of creating a new result identity.
func (r *SessionOperationRepo) OperationForEffect(ctx context.Context, tx pgx.Tx, userID, epoch int64) (*SessionOperation, error) {
	return scanSessionOperation(tx.QueryRow(ctx, `SELECT `+sessionOpColumns+`
		FROM user_session_operations WHERE target_user_id=$1 AND result_epoch=$2 AND status='completed'
		ORDER BY id LIMIT 1`, userID, epoch))
}

// HasAnyIntentForUser 判定账号是否已切入新 schema（存在投影意图即视为已切换；
// 运行时绕过服务层直建的账号保持旧语义，直到首个安全写/bootstrap 触达）。
func (r *SessionOperationRepo) HasAnyIntentForUser(ctx context.Context, userID int64) (bool, error) {
	var ok bool
	err := r.db.QueryRow(ctx,
		`SELECT EXISTS(SELECT 1 FROM session_projection_intents WHERE user_id = $1)`, userID).Scan(&ok)
	return ok, err
}

// PendingIntentCountForUser 当前待处理意图数（含未确认）。
func (r *SessionOperationRepo) PendingIntentCountForUser(ctx context.Context, userID int64) (int, error) {
	var n int
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM session_projection_intents WHERE user_id = $1 AND status = 'pending'`, userID).Scan(&n)
	return n, err
}

// InsertIntentTx 在安全事务内写待同步记录（receipt/intent 同 tx 是恢复必需）。
func (r *SessionOperationRepo) InsertIntentTx(ctx context.Context, tx pgx.Tx, in *SessionProjectionIntent) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO session_projection_intents
			(operation_id, user_id, target_epoch, desired_active, reason, status)
		VALUES ($1,$2,$3,$4,$5,'pending')`,
		in.OperationID, in.UserID, in.TargetEpoch, in.DesiredActive, in.Reason)
	if err != nil {
		return fmt.Errorf("insert projection intent: %w", err)
	}
	return nil
}

// EnsureIntentTx 可重入补齐：worker 从 DB 代际差值发现缺项时，以稳定目标+epoch
// 恢复唯一意图（稳定合成 operation_id 保证不重复，不依赖一次性内存队列）。
func (r *SessionOperationRepo) EnsureIntentTx(ctx context.Context, tx pgx.Tx, op *SessionOperation, in *SessionProjectionIntent) error {
	_, err := tx.Exec(ctx, insertSessionOperation+` ON CONFLICT (operation_id) DO NOTHING`,
		op.OperationID, op.Namespace, op.Command, op.TargetUserID, op.ActorUserID,
		op.ExpectedEpoch, op.InputFingerprint, op.Status, op.ResultEpoch)
	if err != nil {
		return fmt.Errorf("ensure session operation: %w", err)
	}
	var matches bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM user_session_operations
		WHERE operation_id=$1 AND namespace=$2 AND command=$3 AND target_user_id=$4
		AND actor_user_id=$5 AND expected_epoch=$6 AND input_fingerprint=$7
		AND status=$8 AND result_epoch IS NOT DISTINCT FROM $9)`,
		op.OperationID, op.Namespace, op.Command, op.TargetUserID, op.ActorUserID,
		op.ExpectedEpoch, op.InputFingerprint, op.Status, op.ResultEpoch).Scan(&matches); err != nil {
		return fmt.Errorf("verify original session operation: %w", err)
	}
	if !matches || in.OperationID != op.OperationID {
		return errcode.ErrConflict
	}
	_, err = tx.Exec(ctx, `
		INSERT INTO session_projection_intents
			(operation_id, user_id, target_epoch, desired_active, reason, status)
		VALUES ($1,$2,$3,$4,$5,'pending')
		ON CONFLICT (operation_id) DO NOTHING`,
		in.OperationID, in.UserID, in.TargetEpoch, in.DesiredActive, in.Reason)
	if err != nil {
		return fmt.Errorf("ensure projection intent: %w", err)
	}
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM session_projection_intents
		WHERE operation_id=$1 AND user_id=$2 AND target_epoch=$3 AND desired_active=$4)`,
		in.OperationID, in.UserID, in.TargetEpoch, in.DesiredActive).Scan(&matches); err != nil {
		return fmt.Errorf("verify original projection intent: %w", err)
	}
	if !matches {
		return errcode.ErrConflict
	}
	return nil
}

// PendingIntents 待投影意图列表（worker 输入，按 id 稳定序）。
func (r *SessionOperationRepo) PendingIntents(ctx context.Context, limit int) ([]*SessionProjectionIntent, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, operation_id, user_id, target_epoch, desired_active, reason,
		       lease_token, lease_expires_at, fence, attempts, last_error, status
		FROM session_projection_intents WHERE status = 'pending'
		ORDER BY id ASC LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("list pending intents: %w", err)
	}
	defer rows.Close()
	out := []*SessionProjectionIntent{}
	for rows.Next() {
		var in SessionProjectionIntent
		if err := rows.Scan(&in.ID, &in.OperationID, &in.UserID, &in.TargetEpoch,
			&in.DesiredActive, &in.Reason, &in.LeaseToken, &in.LeaseExpiresAt,
			&in.Fence, &in.Attempts, &in.LastError, &in.Status); err != nil {
			return nil, err
		}
		out = append(out, &in)
	}
	return out, rows.Err()
}

// AcquireIntentLease 领取意图租约（fence 单调递增；过期租约可被重新领取——
// 防止过期执行者覆盖结果由 fence CAS 保证）。
func (r *SessionOperationRepo) AcquireIntentLease(ctx context.Context, intentID int64, leaseToken string, ttl time.Duration) (fence int64, ok bool, err error) {
	err = r.db.QueryRow(ctx, `
		UPDATE session_projection_intents SET
			lease_token = $2, lease_expires_at = NOW() + make_interval(secs => $3),
			fence = fence + 1, attempts = attempts + 1, updated_at = NOW()
		WHERE id = $1 AND status = 'pending'
		  AND (lease_expires_at IS NULL OR lease_expires_at < NOW())
		RETURNING fence`, intentID, leaseToken, int64(ttl.Seconds())).Scan(&fence)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("acquire intent lease: %w", err)
	}
	return fence, true, nil
}

// ConfirmIntent 按原 lease/fence CAS 回写确认：投影_epoch 只向前推进，
// 过期/被接管的租约不得覆盖结果。
func (r *SessionOperationRepo) ConfirmIntent(ctx context.Context, intentID int64, leaseToken string, fence, epoch int64) (confirmed bool, err error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin confirm tx: %w", err)
	}
	defer tx.Rollback(ctx)
	// Account-changing commands lock the user before writing its intent. Keep the
	// same lock order and derive both effects from the row that was actually leased.
	var userID, currentEpoch int64
	var active bool
	err = tx.QueryRow(ctx, `
		SELECT u.id, u.session_epoch, u.status=1 AND u.deleted_at IS NULL
		FROM users u JOIN session_projection_intents i ON i.user_id=u.id
		WHERE i.id=$1 FOR UPDATE OF u`, intentID).Scan(&userID, &currentEpoch, &active)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && currentEpoch != epoch) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock projection account: %w", err)
	}
	var targetEpoch int64
	err = tx.QueryRow(ctx, `
		UPDATE session_projection_intents SET status='confirmed', last_error='', updated_at=clock_timestamp()
		WHERE id=$1 AND lease_token=$2 AND fence=$3 AND status='pending'
		AND lease_expires_at > clock_timestamp() AND target_epoch=$4 AND desired_active=$5
		RETURNING target_epoch`, intentID, leaseToken, fence, currentEpoch, active).Scan(&targetEpoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("confirm intent: %w", err)
	}
	tag, err := tx.Exec(ctx, `UPDATE users SET session_projection_epoch=$2
		WHERE id=$1 AND session_epoch=$2 AND session_projection_epoch <= $2`, userID, targetEpoch)
	if err != nil {
		return false, fmt.Errorf("advance projection epoch: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return false, nil
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit confirm: %w", err)
	}
	return true, nil
}

// FailIntent 记录失败原因并释放租约（保持 pending 供重投）。
func (r *SessionOperationRepo) FailIntent(ctx context.Context, intentID int64, leaseToken, _ string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE session_projection_intents SET last_error = 'projection_failed', lease_token = '',
		       lease_expires_at = NULL, updated_at = NOW()
		WHERE id = $1 AND lease_token = $2 AND status = 'pending'
		AND lease_expires_at > clock_timestamp()`, intentID, leaseToken)
	return err
}

// SupersedeIntentsBefore 旧代意图标记被取代（worker 仅按当前 DB 状态投影；
// 旧确认或清理不得覆盖新代）。
func (r *SessionOperationRepo) SupersedeIntentsBefore(ctx context.Context, userID, epoch int64) (int64, error) {
	tag, err := r.db.Exec(ctx, `
		UPDATE session_projection_intents i SET status = 'superseded', updated_at = NOW()
		FROM users u WHERE u.id=$1 AND u.session_epoch=$2
		AND i.user_id=u.id AND i.target_epoch<u.session_epoch AND i.status='pending'`, userID, epoch)
	if err != nil {
		return 0, fmt.Errorf("supersede intents: %w", err)
	}
	return tag.RowsAffected(), nil
}

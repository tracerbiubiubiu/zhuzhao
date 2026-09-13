package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
)

// JobSubmission 任务提交日志 + 回调幂等行（000021，一表两用——E-②，16 号 §3）。
type JobSubmission struct {
	ID          int64
	TaskID      string
	RequestID   string
	Action      string
	Params      string
	Origin      string // api（zhuzhao 提交）/ callback（到达时补录，含 cron 触发）
	Status      string // submitted / running / succeeded / failed
	Error       string
	SubmittedBy string
	SourceIP    string
	CreatedAt   time.Time
	ExecutedAt  *time.Time
	ClaimedAt   *time.Time
}

// 回调幂等状态。
const (
	JobStatusSubmitted = "submitted"
	JobStatusRunning   = "running" // 在途抢占：已取得执行权、副作用执行中（未终态）
	JobStatusSucceeded = "succeeded"
	JobStatusFailed    = "failed"
)

var ErrJobSubmissionNotFound = errors.New("repository: job submission not found")

// JobSubmissionRepo job_submissions 仓储。
type JobSubmissionRepo struct {
	db *pgxpool.Pool
}

func NewJobSubmissionRepo(db *pgxpool.Pool) *JobSubmissionRepo {
	return &JobSubmissionRepo{db: db}
}

// RecordSubmit zhuzhao 侧提交凭证（E-④ 调用）：action + task_id + request_id + params 快照落档。
// task_id 冲突（调用方幂等重提）返回 nil 已存在行，不算错误。
func (r *JobSubmissionRepo) RecordSubmit(ctx context.Context, action, taskID, submittedBy, sourceIP, params string) (*JobSubmission, error) {
	row := &JobSubmission{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO job_submissions (task_id, request_id, action, params, origin, status, submitted_by, source_ip)
		VALUES ($1, NULLIF($2, ''), $3, COALESCE(NULLIF($4, ''), '{}'), 'api', $5, NULLIF($6, ''), NULLIF($7, ''))
		ON CONFLICT (task_id) DO NOTHING
		RETURNING id, task_id, COALESCE(request_id, ''), action, origin, status,
		          COALESCE(error, ''), COALESCE(submitted_by, ''), COALESCE(source_ip, ''), created_at, executed_at`,
		taskID, reqid.From(ctx), action, params, JobStatusSubmitted, submittedBy, sourceIP).Scan(
		&row.ID, &row.TaskID, &row.RequestID, &row.Action, &row.Origin, &row.Status,
		&row.Error, &row.SubmittedBy, &row.SourceIP, &row.CreatedAt, &row.ExecutedAt)
	if err != nil {
		return nil, fmt.Errorf("record job submit: %w", err)
	}
	return row, nil
}

// GetByTaskID 查行（幂等判定入口）。
func (r *JobSubmissionRepo) GetByTaskID(ctx context.Context, taskID string) (*JobSubmission, error) {
	row := &JobSubmission{}
	err := r.db.QueryRow(ctx, `
		SELECT id, task_id, COALESCE(request_id, ''), action, params, origin, status,
		       COALESCE(error, ''), COALESCE(submitted_by, ''), COALESCE(source_ip, ''), created_at, executed_at
		FROM job_submissions WHERE task_id = $1`, taskID).Scan(
		&row.ID, &row.TaskID, &row.RequestID, &row.Action, &row.Params, &row.Origin, &row.Status,
		&row.Error, &row.SubmittedBy, &row.SourceIP, &row.CreatedAt, &row.ExecutedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobSubmissionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job submission: %w", err)
	}
	return row, nil
}

// ClaimCallbackRow 回调到达时的幂等栅栏——单语句**原子抢占**（P1-1）。
//
// 契约（第二个返回值 claimed = 本次是否抢到执行权）：
//   - claimed=true：无行（cron 触发/未落凭证）经 INSERT 补录，或已有行处于
//     submitted / failed（可重试），或 running 但 claimed_at 早于 10 分钟
//     （陈旧——进程崩溃自愈，允许重认领）。抢占成功后本行 status='running'、
//     claimed_at=NOW()；**调用方取得执行权、须自行执行副作用并 Mark* 终态**。
//   - claimed=false：已有行且 status=succeeded（幂等拦截，终态），或 running
//     且 claimed_at 未超 10 分钟（他人正在途执行）。此时本方法**再查回该行**
//     一并返回（供调用方映射响应消息）；调用方**不得执行任何副作用**。
//
// 原子性保证：INSERT ... ON CONFLICT DO UPDATE ... WHERE + RETURNING 在单条语句内
// 完成「判可抢占 + 抢占」，并发同 task_id 恰好 1 个语句返回行——修复前两段式
// （INSERT DO NOTHING + 独立 SELECT）在并发下全部读到 submitted、handler 重复执行。
func (r *JobSubmissionRepo) ClaimCallbackRow(ctx context.Context, taskID, action, actor, sourceIP, params string) (*JobSubmission, bool, error) {
	row := &JobSubmission{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO job_submissions (task_id, request_id, action, params, origin, status, submitted_by, source_ip, claimed_at)
		VALUES ($1, NULLIF($2, ''), $3, COALESCE(NULLIF($4, ''), '{}'), 'callback', 'running', NULLIF($5, ''), NULLIF($6, ''), NOW())
		ON CONFLICT (task_id) DO UPDATE
			SET status = 'running', claimed_at = NOW(), error = ''
			WHERE job_submissions.status IN ('submitted', 'failed')
			   OR (job_submissions.status = 'running'
			       AND job_submissions.claimed_at IS NOT NULL
			       AND job_submissions.claimed_at < NOW() - INTERVAL '10 minutes')
		RETURNING id, task_id, COALESCE(request_id, ''), action, params, origin, status,
		          COALESCE(error, ''), COALESCE(submitted_by, ''), COALESCE(source_ip, ''), created_at, executed_at, claimed_at`,
		taskID, reqid.From(ctx), action, params, actor, sourceIP).Scan(
		&row.ID, &row.TaskID, &row.RequestID, &row.Action, &row.Params, &row.Origin, &row.Status,
		&row.Error, &row.SubmittedBy, &row.SourceIP, &row.CreatedAt, &row.ExecutedAt, &row.ClaimedAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 未抢到执行权：他人已 succeeded（终态），或他人正在途 running（未超 10 分钟）。
			// 查回该行供调用方映射响应；行必然存在（本次 ON CONFLICT 命中）。
			existing, getErr := r.GetByTaskID(ctx, taskID)
			if getErr != nil {
				return nil, false, getErr
			}
			return existing, false, nil
		}
		return nil, false, fmt.Errorf("claim callback row: %w", err)
	}
	return row, true, nil
}

// MarkSucceeded 执行成功（终态；此后同 task_id 回调被幂等拦截）。
// claimedAt 认领令牌（fence）：谓词等值匹配本执行认领时的 claimed_at——
// 10 分钟陈旧重认领后，过期执行的终态写入被拒（updated=false），不覆盖新执行。
func (r *JobSubmissionRepo) MarkSucceeded(ctx context.Context, taskID string, claimedAt time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE job_submissions SET status=$2, error='', executed_at=NOW()
		 WHERE task_id=$1 AND status='running' AND claimed_at=$3`,
		taskID, JobStatusSucceeded, claimedAt)
	if err != nil {
		return false, fmt.Errorf("mark job succeeded: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

// MarkFailed 执行失败（非终态：留 executed_at NULL 允许重试；error 供排障）。
// fence 语义同 MarkSucceeded。
func (r *JobSubmissionRepo) MarkFailed(ctx context.Context, taskID, errMsg string, claimedAt time.Time) (bool, error) {
	tag, err := r.db.Exec(ctx,
		`UPDATE job_submissions SET status=$2, error=$3
		 WHERE task_id=$1 AND status='running' AND claimed_at=$4`,
		taskID, JobStatusFailed, errMsg, claimedAt)
	if err != nil {
		return false, fmt.Errorf("mark job failed: %w", err)
	}
	return tag.RowsAffected() > 0, nil
}

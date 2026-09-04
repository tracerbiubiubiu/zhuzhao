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
	Origin      string // api（zhuzhao 提交）/ callback（到达时补录，含 cron 触发）
	Status      string // submitted / succeeded / failed
	Error       string
	SubmittedBy string
	SourceIP    string
	CreatedAt   time.Time
	ExecutedAt  *time.Time
}

// 回调幂等状态。
const (
	JobStatusSubmitted = "submitted"
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

// RecordSubmit zhuzhao 侧提交凭证（E-④ 调用）：action + task_id + request_id 落档（薄）。
// task_id 冲突（调用方幂等重提）返回 nil 已存在行，不算错误。
func (r *JobSubmissionRepo) RecordSubmit(ctx context.Context, action, taskID, submittedBy, sourceIP string) (*JobSubmission, error) {
	row := &JobSubmission{}
	err := r.db.QueryRow(ctx, `
		INSERT INTO job_submissions (task_id, request_id, action, origin, status, submitted_by, source_ip)
		VALUES ($1, NULLIF($2, ''), $3, 'api', $4, NULLIF($5, ''), NULLIF($6, ''))
		ON CONFLICT (task_id) DO NOTHING
		RETURNING id, task_id, COALESCE(request_id, ''), action, origin, status,
		          COALESCE(error, ''), COALESCE(submitted_by, ''), COALESCE(source_ip, ''), created_at, executed_at`,
		taskID, reqid.From(ctx), action, JobStatusSubmitted, submittedBy, sourceIP).Scan(
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
		SELECT id, task_id, COALESCE(request_id, ''), action, origin, status,
		       COALESCE(error, ''), COALESCE(submitted_by, ''), COALESCE(source_ip, ''), created_at, executed_at
		FROM job_submissions WHERE task_id = $1`, taskID).Scan(
		&row.ID, &row.TaskID, &row.RequestID, &row.Action, &row.Origin, &row.Status,
		&row.Error, &row.SubmittedBy, &row.SourceIP, &row.CreatedAt, &row.ExecutedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrJobSubmissionNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get job submission: %w", err)
	}
	return row, nil
}

// EnsureCallbackRow 回调到达时的幂等栅栏：
//   - 无行（cron 触发/未落凭证）→ 补录 origin='callback' 行，返回 (row, false, nil)；
//   - 已有行且 status=succeeded → 返回 (row, true, nil)——**调用方不得再执行**（幂等拦截）；
//   - 已有行且 submitted/failed → 返回 (row, false, nil)，允许（重）执行。
func (r *JobSubmissionRepo) EnsureCallbackRow(ctx context.Context, taskID, action, actor, sourceIP string) (*JobSubmission, bool, error) {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO job_submissions (task_id, request_id, action, origin, status, submitted_by, source_ip)
		VALUES ($1, NULLIF($2, ''), $3, 'callback', $4, NULLIF($5, ''), NULLIF($6, ''))
		ON CONFLICT (task_id) DO NOTHING`,
		taskID, reqid.From(ctx), action, JobStatusSubmitted, actor, sourceIP); err != nil {
		return nil, false, fmt.Errorf("ensure callback row: %w", err)
	}
	row, err := r.GetByTaskID(ctx, taskID)
	if err != nil {
		return nil, false, err
	}
	return row, row.Status == JobStatusSucceeded, nil
}

// MarkSucceeded 执行成功（终态；此后同 task_id 回调被幂等拦截）。
func (r *JobSubmissionRepo) MarkSucceeded(ctx context.Context, taskID string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE job_submissions SET status=$2, error='', executed_at=NOW() WHERE task_id=$1`,
		taskID, JobStatusSucceeded)
	if err != nil {
		return fmt.Errorf("mark job succeeded: %w", err)
	}
	return nil
}

// MarkFailed 执行失败（非终态：留 executed_at NULL 允许重试；error 供排障）。
func (r *JobSubmissionRepo) MarkFailed(ctx context.Context, taskID, errMsg string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE job_submissions SET status=$2, error=$3 WHERE task_id=$1`,
		taskID, JobStatusFailed, errMsg)
	if err != nil {
		return fmt.Errorf("mark job failed: %w", err)
	}
	return nil
}

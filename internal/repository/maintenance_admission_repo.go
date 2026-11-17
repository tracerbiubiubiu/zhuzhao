package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
)

var ErrMaintenanceAdmissionNotFound = errors.New("maintenance admission not found")

type MaintenanceAdmission struct {
	TaskID, Action, Params, Origin string
	PrincipalUserID                int64
	JobID, DefinitionSHA256        string
	RetentionDays                  int
	ApprovalID, Reason             string
	ValidFrom, ValidUntil          *time.Time
}

type MaintenanceAdmissionRepo struct{ db *pgxpool.Pool }

func NewMaintenanceAdmissionRepo(db *pgxpool.Pool) *MaintenanceAdmissionRepo {
	return &MaintenanceAdmissionRepo{db: db}
}

// Put is insert-only. Reusing a task ID cannot rewrite its original authority;
// the caller must compare the returned fact before any downstream invocation.
func (r *MaintenanceAdmissionRepo) Put(ctx context.Context, a *MaintenanceAdmission) (*MaintenanceAdmission, error) {
	_, err := r.db.Exec(ctx, `INSERT INTO maintenance_task_admissions
 (task_id,action,params,origin,principal_user_id,job_id,definition_sha256,retention_days,approval_id,reason,valid_from,valid_until,request_id)
 VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13) ON CONFLICT(task_id) DO NOTHING`, a.TaskID, a.Action, a.Params, a.Origin, a.PrincipalUserID, a.JobID, a.DefinitionSHA256, a.RetentionDays, a.ApprovalID, a.Reason, a.ValidFrom, a.ValidUntil, reqid.From(ctx))
	if err != nil {
		return nil, err
	}
	return r.Get(ctx, a.TaskID)
}
func (r *MaintenanceAdmissionRepo) Get(ctx context.Context, id string) (*MaintenanceAdmission, error) {
	a := &MaintenanceAdmission{}
	err := r.db.QueryRow(ctx, `SELECT task_id,action,params,origin,principal_user_id,job_id,definition_sha256,retention_days,approval_id,reason,valid_from,valid_until FROM maintenance_task_admissions WHERE task_id=$1`, id).Scan(&a.TaskID, &a.Action, &a.Params, &a.Origin, &a.PrincipalUserID, &a.JobID, &a.DefinitionSHA256, &a.RetentionDays, &a.ApprovalID, &a.Reason, &a.ValidFrom, &a.ValidUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrMaintenanceAdmissionNotFound
	}
	return a, err
}

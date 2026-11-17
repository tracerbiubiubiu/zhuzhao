package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// fixedAuditArchiveJob is a startup-only capability. The application binds the
// fixed service principal to its reviewed archive definition, not an actor label
// supplied by a task or callback. It must not be registered as a public action.
type fixedAuditArchiveJob struct {
	job *AuditArchiveJob
}

// NewFixedAuditArchiveJob constructs the in-process cron's fixed maintenance
// capability. Only explicitly enabled, valid startup configuration can create it;
// the separate callback action still requires TaskActionPolicy admission.
func NewFixedAuditArchiveJob(repo *repository.AuditLogRepo, cfg config.ArchiveConfig, logger *slog.Logger) (ReentrantJob, error) {
	if !cfg.CronEnabled {
		return nil, fmt.Errorf("audit_archive: fixed background maintenance is not enabled")
	}
	if err := cfg.ValidateFixedCron(); err != nil {
		return nil, err
	}
	job := NewAuditArchiveJob(repo, cfg.RetentionDays, cfg.BatchRows, cfg.OutDir, cfg.FileRetentionDays, logger)
	job.logger.Info("audit_archive: fixed background maintenance admitted",
		"principal", "service:zhuzhao/audit_archive", "origin", "in_process_cron", "retention_days", cfg.RetentionDays)
	return &fixedAuditArchiveJob{job: job}, nil
}

func (j *fixedAuditArchiveJob) Handle(ctx context.Context, params json.RawMessage) error {
	parsed, err := decodeArchiveParams(params)
	if err != nil || parsed.ApprovalID != "" || parsed.RetentionDays != 0 && parsed.RetentionDays != j.job.retentionDays {
		return fmt.Errorf("audit_archive: fixed background parameters cannot be overridden: %w", jobs.ErrAbort)
	}
	return j.job.handleWithTx(ctx, j.job.retentionDays, nil)
}

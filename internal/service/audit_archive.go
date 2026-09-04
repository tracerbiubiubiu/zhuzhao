package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// AuditArchiveJob B11② 审计归档（03-audit-l2 §4；taskrunner M3 首个预置动作）。
// 每日 cron 由 taskrunner 侧定义并回调（action_id = "audit_archive"）。
//
// 语义（P4/P7 定案）：
//   - 超期行（created_at < NOW() - retention）导出 JSONL 到本地目录（卷挂载持久）；
//   - **单批导出成功 → 按同批 id 删行**（防「删了没导出」）；崩溃窗口 = 批内：
//     已写文件未删行，下次重跑重复导出该批（跨文件重复可容忍，记录不丢优先）；
//   - 单表失败跳过继续另一表（03 §4.2）；任一表失败 → 返回 error（5xx，taskrunner
//     重试；重入安全：已归档行已删，重跑自然幂等）；
//   - params 可选 {"retention_days": n} 覆盖保留期（排障/回填用）；非法值 → ErrAbort。
type AuditArchiveJob struct {
	repo          *repository.AuditLogRepo
	retentionDays int
	batchRows     int
	outDir        string
	logger        *slog.Logger
}

func NewAuditArchiveJob(repo *repository.AuditLogRepo, retentionDays, batchRows int, outDir string, logger *slog.Logger) *AuditArchiveJob {
	if retentionDays <= 0 {
		retentionDays = 180
	}
	if batchRows <= 0 {
		batchRows = 5000
	}
	if outDir == "" {
		outDir = "data/archive"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &AuditArchiveJob{repo: repo, retentionDays: retentionDays, batchRows: batchRows, outDir: outDir, logger: logger}
}

type archiveParams struct {
	RetentionDays int `json:"retention_days"`
}

// archiveTables 归档顺序（先大表后大表——两张均为大表，顺序无业务含义，仅稳定遍历）。
var archiveTablesOrdered = []string{"audit_logs", "policy_evaluation_logs"}

// Handle 执行一次归档。params 为空时全用任务配置。
func (j *AuditArchiveJob) Handle(ctx context.Context, params json.RawMessage) error {
	retention := j.retentionDays
	if len(params) > 0 {
		var p archiveParams
		if err := json.Unmarshal(params, &p); err != nil {
			return fmt.Errorf("audit_archive: params 解析失败: %v: %w", err, jobs.ErrAbort)
		}
		if p.RetentionDays > 0 {
			retention = p.RetentionDays
		}
	}
	cutoff := time.Now().AddDate(0, 0, -retention)

	if err := os.MkdirAll(j.outDir, 0o755); err != nil {
		return fmt.Errorf("audit_archive: 建目录失败: %w", err)
	}

	var firstErr error
	for _, table := range archiveTablesOrdered {
		exported, deleted, err := j.archiveTable(ctx, table, cutoff)
		if err != nil {
			j.logger.Error("audit_archive: table failed",
				slog.String("table", table), slog.Any("err", err))
			if firstErr == nil {
				firstErr = err
			}
			continue // 单表失败跳过，不阻塞另一表（03 §4.2）
		}
		if exported > 0 {
			j.logger.Info("audit_archive: table done",
				slog.String("table", table), slog.Int64("exported", exported),
				slog.Int64("deleted", deleted), slog.Int("retention_days", retention))
		}
	}
	return firstErr
}

// archiveTable 单表分批归档：写文件（含 flush）→ 删同批行，循环至无超期行。
func (j *AuditArchiveJob) archiveTable(ctx context.Context, table string, cutoff time.Time) (exported, deleted int64, err error) {
	runStamp := time.Now().Format("20060102-150405")
	path := filepath.Join(j.outDir, fmt.Sprintf("%s-%s.jsonl", table, runStamp))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()

	for {
		select {
		case <-ctx.Done():
			return exported, deleted, ctx.Err()
		default:
		}
		batch, err := j.repo.ArchiveFetchBatch(ctx, table, cutoff, j.batchRows)
		if err != nil {
			return exported, deleted, err
		}
		if len(batch) == 0 {
			return exported, deleted, nil
		}
		ids := make([]int64, 0, len(batch))
		for _, row := range batch {
			if _, err := f.Write(append(row.Line, '\n')); err != nil {
				return exported, deleted, fmt.Errorf("write %s: %w", path, err)
			}
			ids = append(ids, row.ID)
		}
		// 文件先落盘（含缓冲冲刷）再删行——崩溃窗口内最多重复导出，不会丢
		if err := f.Sync(); err != nil {
			return exported, deleted, fmt.Errorf("sync %s: %w", path, err)
		}
		if err := j.repo.ArchiveDeleteBatch(ctx, table, ids); err != nil {
			return exported, deleted, err
		}
		exported += int64(len(batch))
		deleted += int64(len(ids))
	}
}

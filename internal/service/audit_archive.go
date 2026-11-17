package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// AuditArchiveJob B11② 审计归档（03-audit-l2 §4）。
//
// P4 cron 收归批（2026-09-29）：触发双形态——① 进程内 CronScheduler（ticker+
// advisory lock，配置开关）② 既有 taskrunner 回调（action_id = "audit_archive"，
// 部署侧二选一防双跑——收归启用即停 taskrunner 侧 job 定义）。
// B-1 修法 c：**单批一执行**（去 for 循环——长任务窗口消除：单批秒级有界，
// 剩余超期行由下个 tick/下次回调继续；空批=快速空过，tick 常跑无需频次控制）。
// B-1 修法 d：本 Handler 可重入（幂等——单批内「先导出后删」+重跑自然续批）。
//
// 语义（P4/P7 定案）：
//   - 超期行（created_at < NOW() - retention）导出 JSONL 到本地目录（卷挂载持久）；
//   - **单批导出成功 → 按同批 id 删行**（防「删了没导出」）；崩溃窗口 = 批内：
//     已写文件未删行，下次重跑重复导出该批（跨文件重复可容忍，记录不丢优先）；
//   - 单表失败跳过继续另一表（03 §4.2）；任一表失败 → 返回 error（5xx，taskrunner
//     重试；重入安全：已归档行已删，重跑自然幂等）；
//   - 默认/零保留期沿固定配置；临时覆盖需私有策略准入上下文，外部JSON不能自授批准。
//     严格参数非法或无批准覆盖 → ErrAbort；受权callback最终资格与DB删除同事务。
type AuditArchiveJob struct {
	repo              *repository.AuditLogRepo
	retentionDays     int
	batchRows         int
	outDir            string
	fileRetentionDays int
	logger            *slog.Logger
}

func NewAuditArchiveJob(repo *repository.AuditLogRepo, retentionDays, batchRows int, outDir string, fileRetentionDays int, logger *slog.Logger) *AuditArchiveJob {
	if retentionDays <= 0 {
		retentionDays = 180
	}
	if batchRows <= 0 {
		batchRows = 5000
	}
	if fileRetentionDays <= 0 {
		fileRetentionDays = 395 // >180 天库内归档口径，留余量
	}
	if outDir == "" {
		outDir = "data/archive"
	}
	if logger == nil {
		logger = slog.Default()
	}
	// 相对 out_dir 在构造时钉死为绝对（基准=进程 cwd，与运行期解析语义一致）并打
	// 启动日志——归档 JSONL 含审计个人信息，随启动目录漂移会落错位置，且磁盘保留
	// 期清理（cleanExpiredArchiveFiles）只扫本目录，错位产物成永久滞留
	// （2026-11-02 taskrunner 孤儿库同类隐患治理）。
	if abs, err := filepath.Abs(outDir); err != nil {
		logger.Warn("audit_archive: out_dir 解析绝对路径失败，沿用原值",
			slog.String("dir", outDir), slog.Any("err", err))
	} else {
		outDir = abs
	}
	logger.Info("audit_archive: 归档目录", slog.String("dir", outDir))
	return &AuditArchiveJob{repo: repo, retentionDays: retentionDays, batchRows: batchRows, outDir: outDir, fileRetentionDays: fileRetentionDays, logger: logger}
}

// cleanExpiredArchiveFiles 清理超过保留期的归档 JSONL 文件（磁盘有界）。
// 失败仅告警不阻塞；文件含审计个人信息，保留口径 > 180 天库内归档（2026-09-07 拍板）。
func (j *AuditArchiveJob) cleanExpiredArchiveFiles() {
	entries, err := os.ReadDir(j.outDir)
	if err != nil {
		j.logger.Warn("audit_archive: 扫描归档目录失败", slog.String("dir", j.outDir), slog.Any("err", err))
		return
	}
	cutoff := time.Now().AddDate(0, 0, -j.fileRetentionDays)
	removed := 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(cutoff) {
			if rmErr := os.Remove(filepath.Join(j.outDir, e.Name())); rmErr != nil {
				j.logger.Warn("audit_archive: 清理过期归档文件失败", slog.String("file", e.Name()), slog.Any("err", rmErr))
			} else {
				removed++
			}
		}
	}
	if removed > 0 {
		j.logger.Info("audit_archive: 清理过期归档文件", slog.Int("removed", removed))
	}
}

type archiveParams struct {
	RetentionDays int    `json:"retention_days,omitempty"`
	ApprovalID    string `json:"approval_id,omitempty"`
}

// archiveTables 归档顺序（先大表后大表——两张均为大表，顺序无业务含义，仅稳定遍历）。
var archiveTablesOrdered = []string{"audit_logs", "policy_evaluation_logs"}

// Handle 执行一次归档。params 为空时全用任务配置。
func (j *AuditArchiveJob) Handle(ctx context.Context, params json.RawMessage) error {
	parsed, err := decodeArchiveParams(params)
	if err != nil {
		return fmt.Errorf("audit_archive: invalid params: %w", jobs.ErrAbort)
	}
	retention := j.retentionDays
	if parsed.RetentionDays > 0 {
		retention = parsed.RetentionDays
	}
	execution, granted := ctx.Value(maintenanceExecutionKey{}).(maintenanceExecution)
	if !granted && (retention != j.retentionDays || parsed.ApprovalID != "") {
		return fmt.Errorf("audit_archive: unapproved retention override: %w", jobs.ErrAbort)
	}
	if granted && (execution.admission.RetentionDays != retention || execution.admission.ApprovalID != parsed.ApprovalID) {
		return fmt.Errorf("audit_archive: admission mismatch: %w", jobs.ErrAbort)
	}
	if granted {
		err := execution.execute(ctx, func(ctx context.Context, tx pgx.Tx) error { return j.handleWithTx(ctx, retention, tx) })
		if err == nil {
			j.cleanExpiredArchiveFiles()
		}
		return err
	}
	return j.handleWithTx(ctx, retention, nil)
}
func (j *AuditArchiveJob) handleWithTx(ctx context.Context, retention int, tx pgx.Tx) error {
	cutoff := time.Now().AddDate(0, 0, -retention)

	if err := os.MkdirAll(j.outDir, 0o755); err != nil {
		return fmt.Errorf("audit_archive: 建目录失败: %w", err)
	}

	var firstErr error
	for _, table := range archiveTablesOrdered {
		exported, deleted, err := j.archiveTableTx(ctx, tx, table, cutoff)
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
	if tx == nil && firstErr == nil {
		j.cleanExpiredArchiveFiles()
	}
	return firstErr
}

// archiveTable 单表**单批**归档：写文件（含 flush）→ 删同批行 → 返回（B-1 修法 c）。
func (j *AuditArchiveJob) archiveTable(ctx context.Context, table string, cutoff time.Time) (exported, deleted int64, err error) {
	return j.archiveTableTx(ctx, nil, table, cutoff)
}
func (j *AuditArchiveJob) archiveTableTx(ctx context.Context, tx pgx.Tx, table string, cutoff time.Time) (exported, deleted int64, err error) {
	// 审计修复（2026-09-30 P2）：先取批后建文件——空批不再泄漏 0 字节文件
	// （原序每 tick 每表一空文件 ≈5760/天，inode 无界）
	select {
	case <-ctx.Done():
		return exported, deleted, ctx.Err()
	default:
	}
	var batch []repository.ArchiveRow
	if tx != nil {
		batch, err = j.repo.ArchiveFetchBatchTx(ctx, tx, table, cutoff, j.batchRows)
	} else {
		batch, err = j.repo.ArchiveFetchBatch(ctx, table, cutoff, j.batchRows)
	}
	if err != nil {
		return exported, deleted, err
	}
	if len(batch) == 0 {
		return exported, deleted, nil
	}
	runStamp := time.Now().Format("20060102-150405.000000000") // 纳秒：同秒重叠运行不共用文件
	path := filepath.Join(j.outDir, fmt.Sprintf("%s-%s.jsonl", table, runStamp))
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return exported, deleted, fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
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
	if tx != nil {
		err = j.repo.ArchiveDeleteBatchTx(ctx, tx, table, ids)
	} else {
		err = j.repo.ArchiveDeleteBatch(ctx, table, ids)
	}
	if err != nil {
		return exported, deleted, err
	}
	return int64(len(batch)), int64(len(ids)), nil
}

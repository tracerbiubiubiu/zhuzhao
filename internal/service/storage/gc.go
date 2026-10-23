package storage

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// GCJob 孤儿对象回收（B5-1：预签未 confirm 的 pending 对象，05 号 A.3 修正②；
// 2026-10-20 删除口径对齐批增二扫——deleted 态残留对象收敛，05 号 A.4 #4
// 「对象物理删除走 GC」口径闭环）。骑既有 CronScheduler（app 装配侧传入——本包
// 不 import service 根包，域隔离守护合规：Handle 签名结构性满足 ReentrantJob）。
// 收敛语义：单批有界、单条失败不炸整批（失败行保持原态下轮重试）——与
// audit_archive 同款 ReentrantJob 契约。
type GCJob struct {
	repo   *repository.StorageRepo
	client storage.Client
	cfg    *storage.ConfigStore // F-D3：与 wire 三服务共享同一快照存储（KV 改 orphan_gc_days 即时可见）
	logger *slog.Logger
}

func NewGCJob(repo *repository.StorageRepo, client storage.Client, cfg *storage.ConfigStore, logger *slog.Logger) *GCJob {
	return &GCJob{repo: repo, client: client, cfg: cfg, logger: logger}
}

const gcBatchSize = 100

// Handle 两段扫描：
//
//  1. pending 孤儿（超 orphan_gc_days）：**先条件清行、后删对象**（分批审计 F-A1
//     修复 2026-10-21——原「先删对象后清行」在 List 与 Remove 之间撞上并发 confirm
//     会把刚确认的对象物理删掉：行 confirmed 在库、对象 404）。条件删 0 行 = 已被
//     并发 confirm 翻态，让位不动；行删成功而对象删失败 → 补插 deleted 墓碑行走
//     二扫重试（否则行已清、对象残留成无扫描覆盖的泄漏）。两种失败模式均收敛。
//  2. deleted 残留（二扫，2026-10-20）：删附件/弃旧流程 best-effort 物理删失败
//     后的行（含 F-A1 墓碑）——重删对象成功即物理清行；失败保持 deleted 下轮再试
//     （修复前 deleted 态无扫描覆盖=对象永久无主泄漏，S5 评审 P2-1）。
func (g *GCJob) Handle(ctx context.Context, _ json.RawMessage) error {
	cfg := g.cfg.Load()
	before := time.Now().AddDate(0, 0, -cfg.OrphanGCDays)
	rows, err := g.repo.ListExpiredPending(ctx, cfg.Bucket, before, gcBatchSize)
	if err != nil {
		g.logger.Error("storage gc list expired pending", "err", err)
		return err
	}
	for _, f := range rows {
		deleted, err := g.repo.DeleteRowIfPending(ctx, f.ID)
		if err != nil {
			g.logger.Error("storage gc delete pending row", "key", f.ObjectKey, "err", err)
			continue
		}
		if !deleted {
			continue // 并发 confirm 让位：对象已属用户，不动（F-A1）
		}
		if err := g.client.RemoveObject(ctx, f.ObjectKey); err != nil {
			g.logger.Warn("storage gc remove object failed, tombstoned for second sweep", "key", f.ObjectKey, "err", err)
			if terr := g.repo.InsertDeletedTombstone(ctx, f); terr != nil {
				// 行已清且墓碑插入失败 = 对象无扫描覆盖——升级 Error 供人工介入（极低概率双失败）
				g.logger.Error("storage gc tombstone insert failed (object leak, manual cleanup needed)", "key", f.ObjectKey, "err", terr)
			}
		}
	}

	dead, err := g.repo.ListDeleted(ctx, cfg.Bucket, gcBatchSize)
	if err != nil {
		g.logger.Error("storage gc list deleted", "err", err)
		return err
	}
	for _, f := range dead {
		if err := g.client.RemoveObject(ctx, f.ObjectKey); err != nil {
			g.logger.Warn("storage gc retry remove deleted failed", "key", f.ObjectKey, "err", err)
			continue
		}
		if err := g.repo.DeleteRowById(ctx, f.ID); err != nil {
			g.logger.Error("storage gc delete dead row", "key", f.ObjectKey, "err", err)
		}
	}

	// F-4-4 外审修复：第三段——反连接回收（confirmed + 工单域 purpose + 无
	// ticket_attachments 关联行 + 超期 → 软删行+物理删对象）。覆盖 2/3 场景：
	// ①建单前 confirmed 悬挂（binding 永不到来）②删单后残留（CASCADE 清关联
	// 行但对象本体未删）。第三场景「编辑从正文删引用」需 custom_data 引用集
	// 解析——继续挂账（RK-P5-16）。
	orphanThreshold := time.Now().AddDate(0, 0, -cfg.OrphanGCDays*2) // 双倍阈值——confirmed 态比 pending 更保守
	confirmed, err := g.repo.ListConfirmedOrphans(ctx, cfg.Bucket, orphanThreshold, gcBatchSize)
	if err != nil {
		g.logger.Error("storage gc list confirmed orphans", "err", err)
		// 不 return——前两段已收敛，第三段失败不影响主链
	} else {
		for _, f := range confirmed {
			// 软删行（confirmed→deleted）→ 物理删对象 → 二扫物理清行
			if err := g.repo.MarkDeleted(ctx, f.ID); err != nil {
				g.logger.Error("storage gc mark confirmed orphan deleted", "key", f.ObjectKey, "err", err)
				continue
			}
			if err := g.client.RemoveObject(ctx, f.ObjectKey); err != nil {
				g.logger.Warn("storage gc remove confirmed orphan failed, second sweep will retry", "key", f.ObjectKey, "err", err)
			}
		}
	}

	if len(rows)+len(dead)+len(confirmed) > 0 {
		g.logger.Info("storage gc pass",
			"pending_scanned", len(rows), "deleted_rescanned", len(dead),
			"confirmed_orphans", len(confirmed))
	}
	return nil
}

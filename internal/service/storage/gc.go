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
	cfg    *storage.StorageConfig
	logger *slog.Logger
}

func NewGCJob(repo *repository.StorageRepo, client storage.Client, cfg *storage.StorageConfig, logger *slog.Logger) *GCJob {
	return &GCJob{repo: repo, client: client, cfg: cfg, logger: logger}
}

const gcBatchSize = 100

// Handle 两段扫描：
//
//  1. pending 孤儿（超 orphan_gc_days）：先删对象后清行（fail-safe 顺序——清行
//     失败则对象已删、行残留 pending 下轮重删对象[存储侧幂等]；反向则可能行已
//     清而对象残留成无主泄漏）。行物理清除= A.9「对象与行均消失」终态
//     （条件删 pending 态，并发 confirm 翻态时让位不动）。
//  2. deleted 残留（二扫，2026-10-20）：删附件/弃旧流程 best-effort 物理删失败
//     后的行——重删对象成功即物理清行；失败保持 deleted 下轮再试（修复前
//     deleted 态无扫描覆盖=对象永久无主泄漏，S5 评审 P2-1）。
func (g *GCJob) Handle(ctx context.Context, _ json.RawMessage) error {
	before := time.Now().AddDate(0, 0, -g.cfg.OrphanGCDays)
	rows, err := g.repo.ListExpiredPending(ctx, g.cfg.Bucket, before, gcBatchSize)
	if err != nil {
		g.logger.Error("storage gc list expired pending", "err", err)
		return err
	}
	for _, f := range rows {
		if err := g.client.RemoveObject(ctx, f.ObjectKey); err != nil {
			// 单条失败不炸整批：行保持 pending，下轮重试（对象删除幂等）
			g.logger.Warn("storage gc remove object failed", "key", f.ObjectKey, "err", err)
			continue
		}
		if err := g.repo.DeleteRowIfPending(ctx, f.ID); err != nil {
			g.logger.Error("storage gc delete pending row", "key", f.ObjectKey, "err", err)
		}
	}

	dead, err := g.repo.ListDeleted(ctx, g.cfg.Bucket, gcBatchSize)
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

	if len(rows)+len(dead) > 0 {
		g.logger.Info("storage gc pass", "pending_scanned", len(rows), "deleted_rescanned", len(dead))
	}
	return nil
}

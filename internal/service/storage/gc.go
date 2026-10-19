package storage

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// GCJob 孤儿对象回收（B5-1：预签未 confirm 的 pending 对象，05 号 A.3 修正②）。
// 骑既有 CronScheduler（app 装配侧传入——本包不 import service 根包，域隔离守护
// 合规：Handle 签名结构性满足 ReentrantJob）。收敛语义：单批有界、单条失败不炸
// 整批（失败行保持 pending 下轮重试）——与 audit_archive 同款 ReentrantJob 契约。
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

// Handle 扫过期 pending → 删对象 → 落标 deleted（顺序：先删对象后落标——
// 落标失败则对象已删、行残留 pending 下轮重删对象[存储侧幂等]，反向则可能
// 行已删而对象残留成无主泄漏）。
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
		if err := g.repo.MarkDeleted(ctx, f.ID); err != nil {
			g.logger.Error("storage gc mark deleted", "key", f.ObjectKey, "err", err)
			continue
		}
	}
	if len(rows) > 0 {
		g.logger.Info("storage gc pass", "scanned", len(rows))
	}
	return nil
}

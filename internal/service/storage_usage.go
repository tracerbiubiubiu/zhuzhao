package service

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

// StorageUsageSampler C-6（2026-10-27 A-2 拍板 a 案）：file_objects 按 purpose
// SQL 采样进 /metrics + 容量阈值检查。采样不碰存储层——000045 建表即有
// size_bytes/purpose 列，一条 GROUP BY 即得（比对象枚举便宜一个量级；avatar 段
// 泄漏当日显形即依赖本维度）。阈值：capacity_bytes=0 只暴露绝对值不判阈；>0 时
// 80% warn/90% crit——WARN 每轮记录（小时级低频不刷屏），事件仅档位跃迁 emit
// （防持续越限每小时重复投递；管理员未配收件人时段子规则空转无害）。
// 注：phase6 R1 水位系 ingest_sources 表增量拉取机制（01 号 §1），与本采样器
// 非同一基座——勿以「复用」为由合并论证（台账 A-2 评审注记）。
type StorageUsageSampler struct {
	pool     *pgxpool.Pool
	registry *domainevent.HookRegistry
	capacity int64
	logger   *slog.Logger
	level    int // 0=正常 1=warn 2=critical（跃迁沿：升/降均 emit 一次）
}

func NewStorageUsageSampler(pool *pgxpool.Pool, registry *domainevent.HookRegistry, capacityBytes int64, logger *slog.Logger) *StorageUsageSampler {
	return &StorageUsageSampler{pool: pool, registry: registry, capacity: capacityBytes, logger: logger}
}

// Sample 单轮采样（骑 CronScheduler 小时级；ReentrantJob 契约——查库失败上抛
// 供 cronFailEmit 告警，采样零行也写 total=0 保证指标连续）
func (s *StorageUsageSampler) Sample(ctx context.Context) error {
	rows, err := s.pool.Query(ctx, `
		SELECT purpose, COALESCE(SUM(size_bytes), 0) FROM file_objects GROUP BY purpose`)
	if err != nil {
		return fmt.Errorf("storage usage sample: %w", err)
	}
	defer rows.Close()

	var total int64
	perPurpose := map[string]int64{}
	for rows.Next() {
		var p string
		var sum int64
		if err := rows.Scan(&p, &sum); err != nil {
			return fmt.Errorf("storage usage scan: %w", err)
		}
		perPurpose[p] = sum
		total += sum
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("storage usage rows: %w", err)
	}
	for p, sum := range perPurpose {
		middleware.SetGauge("zhuzhao_storage_usage_bytes", p, float64(sum))
	}
	middleware.SetGauge("zhuzhao_storage_usage_bytes", "total", float64(total))

	if s.capacity <= 0 {
		return nil // 容量基线未配——只暴露不判阈（C-6 拍板形态③）
	}
	ratio := float64(total) / float64(s.capacity)
	level := 0
	switch {
	case ratio >= 0.90:
		level = 2
	case ratio >= 0.80:
		level = 1
	}
	if level >= 1 {
		s.logger.Warn("storage: usage above threshold",
			"usage_bytes", total, "capacity_bytes", s.capacity,
			"ratio", fmt.Sprintf("%.2f", ratio), "level", levelName(level))
	}
	if level != s.level {
		// 档位跃迁才 emit（含回落到 0 的恢复通知）；registry 可空——纯采样部署态
		if s.registry != nil {
			s.registry.Dispatch(ctx, domainevent.DomainEvent{
				BizType: domainevent.BizOps,
				Action:  domainevent.ActionOpsStorageUsageHigh,
				BizID:   "storage",
				Payload: domainevent.Payload{
					"level":          levelName(level),
					"usage_bytes":    fmt.Sprintf("%d", total),
					"capacity_bytes": fmt.Sprintf("%d", s.capacity),
					"ratio":          fmt.Sprintf("%.2f", ratio),
				},
			})
		}
		s.level = level
	}
	return nil
}

func levelName(level int) string {
	switch level {
	case 2:
		return "critical"
	case 1:
		return "warn"
	default:
		return "ok"
	}
}

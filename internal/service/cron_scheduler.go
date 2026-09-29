package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// CronScheduler P4 cron 收归（2026-09-29 拍板方向落地）：进程内 ticker + PG
// advisory lock 单实例锁——审计数据与归档内务全在 zhuzhao 本地，闹钟外置
// taskrunner 属既成依赖（所有者质疑成立）。
//
// 语义：
//   - tick（默认 30s）→ pg_try_advisory_lock(lockKey)——拿到才执行（多副本
//     部署天然单实例跑，锁随会话释放——进程退出自动放）；
//   - 业务侧单批有界（audit_archive 单批一执行，B-1 修法 c）——tick 常跑
//     无需频次控制：有超期行逐批收敛、无行快速空过（一次 SELECT 成本）；
//   - 与 taskrunner 侧 cron job 定义**部署侧二选一**（同任务双启=双跑浪费
//     而非错误——advisory lock 只防本仓多副本）；退役 checklist 见 deploy 文档。
type CronScheduler struct {
	pool    *pgxpool.Pool
	lockKey int64
	job     ReentrantJob
	name    string
	tick    time.Duration
	logger  *slog.Logger
}

// ReentrantJob B-1 修法 d：可重入契约升格为接口约束（从 audit_archive 的
// 注释升格——registry 内全部动作必须满足：任意时刻中断/并发/重复执行，
// 结果幂等或可收敛，不得产生重复副作用）。
type ReentrantJob interface {
	Handle(ctx context.Context, params json.RawMessage) error
}

func NewCronScheduler(pool *pgxpool.Pool, name string, job ReentrantJob, tick time.Duration, logger *slog.Logger) *CronScheduler {
	if tick <= 0 {
		tick = 30 * time.Second
	}
	if logger == nil {
		logger = slog.Default()
	}
	// 锁键=任务名稳定散列（跨实例同任务同键；int64 语义空间）
	sum := sha256.Sum256([]byte("zhuzhao:cron:" + name))
	return &CronScheduler{pool: pool, lockKey: int64(binary.BigEndian.Uint64(sum[:8])), job: job, name: name, tick: tick, logger: logger}
}

// Run 阻塞循环（App 生命周期内；ctx 取消即退出）
func (s *CronScheduler) Run(ctx context.Context) {
	s.logger.Info("cron scheduler started", slog.String("job", s.name), slog.Duration("tick", s.tick))
	t := time.NewTicker(s.tick)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			s.tickOnce(ctx)
		}
	}
}

func (s *CronScheduler) tickOnce(ctx context.Context) {
	// 单批有界（B-1 修法 c）——单次执行独立超时兜底（防慢查询挂住 tick 位）
	execCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	conn, err := s.pool.Acquire(execCtx)
	if err != nil {
		s.logger.Warn("cron: acquire conn failed", slog.String("job", s.name), slog.Any("err", err))
		return
	}
	defer conn.Release()

	// pg_try_advisory_lock：非阻塞——他实例在跑则本 tick 跳过
	var ok bool
	if err := conn.QueryRow(execCtx, "SELECT pg_try_advisory_lock($1)", s.lockKey).Scan(&ok); err != nil || !ok {
		return
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), "SELECT pg_advisory_unlock($1)", s.lockKey) }()

	// 审计修复（2026-09-30 P2）：job panic 兜底——Run 在裸 goroutine，panic=进程退出
	func() {
		defer func() {
			if r := recover(); r != nil {
				s.logger.Error("cron: job panicked", slog.String("job", s.name), slog.Any("panic", r))
			}
		}()
		err = s.job.Handle(execCtx, nil)
	}()
	if err != nil {
		// 归档类任务失败仅记日志（下个 tick 自然重试——可重入契约保证安全）
		s.logger.Warn("cron: job failed (will retry next tick)", slog.String("job", s.name), slog.Any("err", err))
	}
}

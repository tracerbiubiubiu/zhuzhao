// Package audit 判定日志 L2 写入管道（B11①，03-audit-l2 §2/§3）。
//
// 数据流（P3 拍板 2026-09-03：异步写）：
//
//	hook（registry.Authorize）→ 有界 channel → pump: RPUSH Redis List
//	  → flusher: LMOVE List→processing → LRANGE → 批量 INSERT → LTRIM processing
//
// 崩溃语义（AL4）：channel 内未推送的行丢失窗口 ≤ 一次 pump 间隔（fail-open 已拍板）；
// 已入 List / processing 的行持久在 Redis（AOF），重启后 flusher 继续消费不丢。
// 落库失败：留在 processing，下轮重试（不丢、不阻塞业务）。
// Redis 不可用：pump 丢弃该行并 Warn——判定日志写失败绝不阻断鉴权（fail-open）。
package audit

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// PolicyEvalEntry 判定日志行（与 policy_evaluation_logs 列对齐）。
type PolicyEvalEntry struct {
	ActorID      int64    `json:"actor_id"`
	ActorRoles   []string `json:"actor_roles,omitempty"`
	ResourceType string   `json:"resource_type"`
	ResourceID   string   `json:"resource_id"`
	Action       string   `json:"action"`
	Result       bool     `json:"result"`
	Reason       string   `json:"reason,omitempty"`
	TraceID      string   `json:"trace_id,omitempty"`
	CreatedAt    string   `json:"created_at"` // RFC3339（JSON 载体格式）
}

// PolicyEvalStore 批量落库（由 repository 实现；writer 不感知 SQL）。
type PolicyEvalStore interface {
	InsertPolicyEvals(ctx context.Context, rows []PolicyEvalEntry) error
}

// PolicyEvalConfig 管道参数（零值取默认：buffer 1024 / batch 200 / flush 1s）。
type PolicyEvalConfig struct {
	BufferSize    int
	BatchSize     int
	FlushInterval time.Duration
	RedisKey      string // 默认 audit:policy_eval
	ProcessingKey string // 默认 <key>:processing
}

func (c PolicyEvalConfig) withDefaults() PolicyEvalConfig {
	out := c
	if out.BufferSize <= 0 {
		out.BufferSize = 1024
	}
	if out.BatchSize <= 0 {
		out.BatchSize = 200
	}
	if out.FlushInterval <= 0 {
		out.FlushInterval = time.Second
	}
	if out.RedisKey == "" {
		out.RedisKey = "audit:policy_eval"
	}
	if out.ProcessingKey == "" {
		out.ProcessingKey = out.RedisKey + ":processing"
	}
	return out
}

// PolicyEvalWriter 判定日志 L2 writer。Write 非阻塞（满则丢弃 + Warn）。
type PolicyEvalWriter struct {
	cfg     PolicyEvalConfig
	rdb     *goredis.Client
	store   PolicyEvalStore
	logger  *slog.Logger
	ch      chan PolicyEvalEntry
	dropped atomic.Int64
}

func NewPolicyEvalWriter(cfg PolicyEvalConfig, rdb *goredis.Client, store PolicyEvalStore, logger *slog.Logger) *PolicyEvalWriter {
	if logger == nil {
		logger = slog.Default()
	}
	cfg = cfg.withDefaults()
	return &PolicyEvalWriter{
		cfg: cfg, rdb: rdb, store: store, logger: logger,
		ch: make(chan PolicyEvalEntry, cfg.BufferSize),
	}
}

// Write 非阻塞入队。channel 满（Redis 长时间不可用导致 pump 堵塞等场景）即丢弃：
// fail-open 拍板——判定日志缺失可接受，鉴权绝不被日志拖挂。
func (w *PolicyEvalWriter) Write(e PolicyEvalEntry) {
	if e.CreatedAt == "" {
		e.CreatedAt = time.Now().Format(time.RFC3339Nano)
	}
	select {
	case w.ch <- e:
	default:
		n := w.dropped.Add(1)
		w.logger.Warn("policy_eval: buffer full, entry dropped (fail-open)",
			slog.Int64("dropped_total", n), slog.String("resource", e.ResourceType))
	}
}

// Start 启动 pump 与 flusher；ctx 取消后尽力排空 channel 再退出（优雅停止）。
func (w *PolicyEvalWriter) Start(ctx context.Context) {
	go w.pump(ctx)
	go w.flusher(ctx)
}

// pump channel → Redis List。推送失败丢弃该行（fail-open）。
// 推送用脱离取消的 ctx：取消只终止循环（退出前经 drain 尽力排空），
// 不毙掉已从 channel 取出的在途条目（否则 cancel 与读同时就绪时静默丢行）。
func (w *PolicyEvalWriter) pump(ctx context.Context) {
	push := func(e PolicyEvalEntry) {
		b, err := json.Marshal(e)
		if err != nil {
			return // 结构体恒可序列化，防御分支
		}
		pctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 3*time.Second)
		defer cancel()
		if err := w.rdb.RPush(pctx, w.cfg.RedisKey, b).Err(); err != nil {
			if ctx.Err() == nil {
				w.logger.Warn("policy_eval: redis push failed, entry dropped (fail-open)",
					slog.Any("err", err))
			}
		}
	}
	for {
		select {
		case <-ctx.Done():
			w.drain()
			return
		case e := <-w.ch:
			push(e)
		}
	}
}

// drain 优雅停止：取消后限时排空 channel 入 Redis（shutdown drain 语义，尽力而为）。
func (w *PolicyEvalWriter) drain() {
	timeout := time.After(3 * time.Second)
	for {
		select {
		case <-timeout:
			return
		case e := <-w.ch:
			b, _ := json.Marshal(e)
			ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), time.Second)
			_ = w.rdb.RPush(ctx, w.cfg.RedisKey, b).Err()
			cancel()
		default:
			return
		}
	}
}

// flusher Redis List → processing → 批量落库。落库失败留在 processing 下轮重试。
func (w *PolicyEvalWriter) flusher(ctx context.Context) {
	t := time.NewTicker(w.cfg.FlushInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			w.flushOnce(context.WithoutCancel(ctx))
			return
		case <-t.C:
			w.flushOnce(ctx)
		}
	}
}

func (w *PolicyEvalWriter) flushOnce(ctx context.Context) {
	for {
		// List → processing（LMOVE 原子搬移：崩溃时不丢——行在 processing 持久）
		for i := 0; i < w.cfg.BatchSize; i++ {
			if err := w.rdb.LMove(ctx, w.cfg.RedisKey, w.cfg.ProcessingKey, "LEFT", "RIGHT").Err(); err != nil {
				if err != goredis.Nil {
					w.logger.Warn("policy_eval: lmove failed, retry next tick", slog.Any("err", err))
				}
				break
			}
		}
		items, err := w.rdb.LRange(ctx, w.cfg.ProcessingKey, 0, int64(w.cfg.BatchSize-1)).Result()
		if err != nil {
			w.logger.Warn("policy_eval: lrange failed, retry next tick", slog.Any("err", err))
			return
		}
		if len(items) == 0 {
			return
		}
		rows := make([]PolicyEvalEntry, 0, len(items))
		for _, it := range items {
			var e PolicyEvalEntry
			if err := json.Unmarshal([]byte(it), &e); err != nil {
				// 载荷损坏：跳过该行（落库前即剔除，随批 LTRIM 移除，防毒丸卡管道）
				continue
			}
			rows = append(rows, e)
		}
		if err := w.store.InsertPolicyEvals(ctx, rows); err != nil {
			w.logger.Warn("policy_eval: insert failed, keep in processing for retry", slog.Any("err", err))
			return
		}
		if err := w.rdb.LTrim(ctx, w.cfg.ProcessingKey, int64(len(items)), -1).Err(); err != nil {
			// 已落库但裁剪失败：下轮会重复插入这些行（判定日志允许重复——
			// 幂等不在本层承诺，量级为 flush 窗口内一批），记日志供对账
			w.logger.Warn("policy_eval: ltrim failed after insert (duplicate window)",
				slog.Int("rows", len(items)), slog.Any("err", err))
		}
		if len(items) < w.cfg.BatchSize {
			return
		}
	}
}

// Dropped 累计丢弃数（监控/对账用）。
func (w *PolicyEvalWriter) Dropped() int64 { return w.dropped.Load() }

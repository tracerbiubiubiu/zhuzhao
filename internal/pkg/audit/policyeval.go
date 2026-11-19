// Package audit 判定日志 L2 写入管道（B11①，03-audit-l2 §2/§3）。
//
// 数据流（P3 拍板 2026-09-03：异步写）：
//
//	hook（registry.Authorize）→ 有界 channel → pump: RPUSH Redis List
//	  → flusher: LMOVE List→processing → 认领快照 → 批量 INSERT → 核对认领后裁剪
//
// 崩溃语义（AL4）：channel 内未推送的行丢失窗口 ≤ 一次 pump 间隔（fail-open 已拍板）；
// 已入 List / processing 的行持久在 Redis（AOF），重启后 flusher 继续消费不丢。
// 落库失败：留在 processing，下轮重试（不丢、不阻塞业务）。
// Redis 不可用：pump 丢弃该行并 Warn——判定日志写失败绝不阻断鉴权（fail-open）。
package audit

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// PolicyEvalEntry 判定日志行（与 policy_evaluation_logs 列对齐）。
type PolicyEvalEntry struct {
	// 内部管道载荷（Redis List → 批量落库），不出站 API——int64 直存无 JS 精度问题；
	// 若未来判定日志开读端点，本字段须按 §3-7 改 `,string` 出站
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
	wg      sync.WaitGroup // pump/flusher 退出等待（Stop 用；防先关 Redis 静默丢行）
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

// Start 启动 pump 与 flusher；ctx 取消时在仍持有 ownerCtx 的前提下尽力排空。
// 退出收尾须由 Stop 等待——否则调用方先关 Redis 时 drain/收尾 flush 静默失败。
func (w *PolicyEvalWriter) Start(ctx, ownerCtx context.Context) {
	pumpDone := make(chan struct{})
	w.wg.Add(2)
	go func() {
		defer w.wg.Done()
		defer close(pumpDone)
		w.pump(ctx, ownerCtx)
	}()
	go func() {
		defer w.wg.Done()
		w.flusher(ctx, ownerCtx, pumpDone)
	}()
}

// Stop 等待 pump/flusher 完成退出前收尾（drain + 收尾 flush）。必须在 ctx 取消
// 之后、Redis 连接关闭之前调用（App.Shutdown 内、server.Shutdown 后）。超时返回
// false 只表示未在预算内等齐：已入 Redis 的行可重启续消；未完成推送的
// channel 行仍受既有 fail-open 窗口约束。true 也不等于最终数据库写入成功。
func (w *PolicyEvalWriter) Stop(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(timeout):
		return false
	}
}

// pump channel → Redis List。推送失败丢弃该行（fail-open）。
// 推送用脱离取消的 ctx：取消只终止循环（退出前经 drain 尽力排空），
// 不毙掉已从 channel 取出的在途条目（否则 cancel 与读同时就绪时静默丢行）。
func (w *PolicyEvalWriter) pump(ctx, ownerCtx context.Context) {
	push := func(e PolicyEvalEntry) {
		if ownerCtx.Err() != nil {
			return
		}
		b, err := json.Marshal(e)
		if err != nil {
			return // 结构体恒可序列化，防御分支
		}
		pctx, cancel := context.WithTimeout(ownerCtx, 3*time.Second)
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
		case <-ownerCtx.Done():
			return
		case <-ctx.Done():
			w.drain(ownerCtx)
			return
		case e := <-w.ch:
			push(e)
		}
	}
}

// drain 优雅停止：取消后限时排空 channel 入 Redis（shutdown drain 语义，尽力而为）。
func (w *PolicyEvalWriter) drain(ownerCtx context.Context) {
	ctx, cancel := context.WithTimeout(ownerCtx, 3*time.Second)
	defer cancel()
	for ctx.Err() == nil {
		select {
		case <-ctx.Done():
			return
		case e := <-w.ch:
			b, _ := json.Marshal(e)
			pushCtx, pushCancel := context.WithTimeout(ctx, time.Second)
			err := w.rdb.RPush(pushCtx, w.cfg.RedisKey, b).Err()
			pushCancel()
			if err != nil {
				// 停机排空失败必须留痕：此处静默丢行 = 优雅停机路径上的无痕丢失
				w.logger.Warn("policy_eval: shutdown drain push failed, entry lost",
					slog.Any("err", err), slog.String("resource", e.ResourceType))
			}
		default:
			return
		}
	}
}

// flusher Redis List → processing → 批量落库。落库失败留在 processing 下轮重试。
func (w *PolicyEvalWriter) flusher(ctx, ownerCtx context.Context, pumpDone <-chan struct{}) {
	t := time.NewTicker(w.cfg.FlushInterval)
	defer t.Stop()
	for {
		select {
		case <-ownerCtx.Done():
			return
		case <-ctx.Done():
			// No producer remains after pump exits. Final flushing must not race
			// ahead of its last push/drain and leave healthy shutdown rows behind.
			select {
			case <-pumpDone:
			case <-ownerCtx.Done():
				return
			}
			fctx, fcancel := context.WithTimeout(ownerCtx, 5*time.Second)
			defer fcancel()
			w.flushOnce(fctx) // 有界收尾：防 Redis 挂起阻塞退出（残留行重启续消）
			return
		case <-t.C:
			w.flushOnce(ctx)
		}
	}
}

// Read the head and claim it in one Redis operation. A successor can re-read
// durable entries immediately, and the superseded consumer cannot trim its batch.
var claimPolicyBatch = goredis.NewScript(`
local items = redis.call('LRANGE', KEYS[1], 0, ARGV[1])
if #items > 0 then redis.call('SET', KEYS[2], ARGV[2]) end
return items
`)

var ackPolicyBatch = goredis.NewScript(`
if redis.call('GET', KEYS[2]) ~= ARGV[1] then return 0 end
redis.call('LTRIM', KEYS[1], ARGV[2], -1)
redis.call('DEL', KEYS[2])
return 1
`)

func (w *PolicyEvalWriter) flushOnce(ctx context.Context) {
	keys := []string{w.cfg.ProcessingKey, w.cfg.ProcessingKey + ":claim"}
	for ctx.Err() == nil {
		// List → processing（LMOVE 原子搬移：崩溃时不丢——行在 processing 持久）
		for i := 0; i < w.cfg.BatchSize; i++ {
			if ctx.Err() != nil {
				return
			}
			if err := w.rdb.LMove(ctx, w.cfg.RedisKey, w.cfg.ProcessingKey, "LEFT", "RIGHT").Err(); err != nil {
				if !errors.Is(err, goredis.Nil) {
					w.logger.Warn("policy_eval: lmove failed, retry next tick", slog.Any("err", err))
				}
				break
			}
		}
		if ctx.Err() != nil {
			return
		}
		claim := rand.Text()
		items, err := claimPolicyBatch.Run(ctx, w.rdb, keys, w.cfg.BatchSize-1, claim).StringSlice()
		if err != nil {
			w.logger.Warn("policy_eval: batch claim failed, retry next tick", slog.Any("err", err))
			return
		}
		if len(items) == 0 || ctx.Err() != nil {
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
		if ctx.Err() != nil {
			return
		}
		acked, err := ackPolicyBatch.Run(ctx, w.rdb, keys, claim, len(items)).Int()
		if err != nil {
			w.logger.Warn("policy_eval: acknowledgement failed after insert, pending rows may replay",
				slog.Int("rows", len(items)), slog.Any("err", err))
			return
		}
		if acked == 0 {
			w.logger.Warn("policy_eval: batch claim changed, stopping stale flush", slog.Int("rows", len(items)))
			return
		}
		if len(items) < w.cfg.BatchSize {
			return
		}
	}
}

// Dropped 累计丢弃数（监控/对账用）。
func (w *PolicyEvalWriter) Dropped() int64 { return w.dropped.Load() }

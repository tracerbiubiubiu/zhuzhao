// S9 B9-1 情报查询服务（09 号附录 A.3 查询代理链路）：
// 归一化 → 查 intel_cache（未过期命中 hit_count++ 直接返回 / 已过期=stale 返回
// +异步回源刷新）→ 未命中 singleflight 回源（同 key 并发仅一次上游调用——
// 防击穿保护上游配额即本件存在意义）→ 入缓存（含 404 查无——clean 答案同样
// 缓存 TTL 内免打上游）→ 429 降级（有过期缓存→stale 返回；无→503）。
package threatintel

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/singleflight"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// TTLConfig 类型级 TTL（A.1：7/30 天按指标衰减特性 config 化）。
type TTLConfig map[string]time.Duration

// Service 情报查询服务。
type Service struct {
	db       *pgxpool.Pool
	provider Provider
	ttl      TTLConfig
	group    singleflight.Group
	logger   *slog.Logger
}

// NewService 创建服务（ttl 缺省 7 天兜底）。
func NewService(db *pgxpool.Pool, provider Provider, ttl TTLConfig, logger *slog.Logger) *Service {
	if logger == nil {
		logger = slog.Default()
	}
	return &Service{db: db, provider: provider, ttl: ttl, logger: logger}
}

// Response 查询端点出站（A.4 契约）。
type Response struct {
	Type      string          `json:"type"`
	Value     string          `json:"value"`
	Result    json.RawMessage `json:"result"`
	Stale     bool            `json:"stale"`
	FetchedAt time.Time       `json:"fetched_at"`
	HitCount  int             `json:"hit_count"`
}

// Query 查询主链路。
func (s *Service) Query(ctx context.Context, intelType, rawValue string) (*Response, error) {
	if !ValidType(intelType) {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "type 须为 ip/domain/hash/url")
	}
	value := Normalize(intelType, rawValue)
	if value == "" || len(value) > 200 {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "value 不能为空且 ≤200 字符")
	}
	ttl := s.ttlOf(intelType)

	// ① 缓存命中路径
	if hit, ok := s.lookup(ctx, intelType, value); ok {
		if !time.Now().After(hit.ExpiresAt) {
			s.bumpHit(ctx, intelType, value)
			hit.HitCount++
			return hit.toResponse(false), nil
		}
		// 懒过期（拍板点 1 预确认）：stale 返回 + 异步回源刷新（失败仅落日志不阻塞）
		go s.refresh(context.Background(), intelType, value, ttl)
		return hit.toResponse(true), nil
	}

	// ② 未命中：singleflight 回源（同 key 并发仅一次上游调用）
	v, err, _ := s.group.Do(intelType+"/"+value, func() (any, error) {
		return s.fetchAndStore(ctx, intelType, value, ttl)
	})
	if err != nil {
		// 429 降级：有（过期）缓存 → stale 返回；无 → 503（A.3）
		if errors.Is(err, ErrIntelUpstreamQuota) {
			if hit, ok := s.lookupIncludingExpired(ctx, intelType, value); ok {
				return hit.toResponse(true), nil
			}
			return nil, ErrIntelUpstreamQuota
		}
		return nil, err
	}
	return v.(*Response), nil
}

type cacheRow struct {
	Result    json.RawMessage
	FetchedAt time.Time
	ExpiresAt time.Time
	HitCount  int
}

func (r cacheRow) toResponse(stale bool) *Response {
	return &Response{
		Type: "", Value: "", Result: r.Result, Stale: stale,
		FetchedAt: r.FetchedAt, HitCount: r.HitCount,
	}
}

// lookup 查缓存（不过滤过期——由调用方分支处理）。
func (s *Service) lookup(ctx context.Context, intelType, value string) (*cacheRow, bool) {
	return s.lookupRow(ctx, intelType, value)
}

// lookupIncludingExpired 显式语义别名（降级路径用）。
func (s *Service) lookupIncludingExpired(ctx context.Context, intelType, value string) (*cacheRow, bool) {
	return s.lookupRow(ctx, intelType, value)
}

func (s *Service) lookupRow(ctx context.Context, intelType, value string) (*cacheRow, bool) {
	var r cacheRow
	err := s.db.QueryRow(ctx,
		`SELECT result, fetched_at, expires_at, hit_count FROM intel_cache WHERE intel_type = $1 AND query_value = $2`,
		intelType, value).Scan(&r.Result, &r.FetchedAt, &r.ExpiresAt, &r.HitCount)
	if err != nil {
		return nil, false // 含 ErrNoRows 与瞬时 DB 错——均走回源路径
	}
	resp := r.toResponse(false)
	resp.Type, resp.Value = intelType, value
	// toResponse 不带 type/value（fetchAndStore 侧统一填）——此处补
	return &cacheRow{Result: r.Result, FetchedAt: r.FetchedAt, ExpiresAt: r.ExpiresAt, HitCount: r.HitCount}, true
}

func (s *Service) bumpHit(ctx context.Context, intelType, value string) {
	_, _ = s.db.Exec(ctx,
		`UPDATE intel_cache SET hit_count = hit_count + 1 WHERE intel_type = $1 AND query_value = $2`,
		intelType, value)
}

// fetchAndStore 回源 + 入缓存（404 查无=clean 答案同样入缓存——深审项 D）。
func (s *Service) fetchAndStore(ctx context.Context, intelType, value string, ttl time.Duration) (*Response, error) {
	data, err := s.provider.Query(ctx, Query{Type: intelType, Value: value})
	var notFound bool
	if err != nil {
		if errors.Is(err, ErrIntelNotFound) {
			notFound = true
			data = json.RawMessage(`{"not_found":true}`)
		} else {
			return nil, err // 429/5xx 走调用方降级/失败链，不入缓存
		}
	}
	now := time.Now().UTC()
	_, serr := s.db.Exec(ctx, `
		INSERT INTO intel_cache (intel_type, query_value, result, fetched_at, expires_at, hit_count)
		VALUES ($1, $2, $3, $4, $5, 1)
		ON CONFLICT (intel_type, query_value)
		DO UPDATE SET result = EXCLUDED.result, fetched_at = EXCLUDED.fetched_at,
			expires_at = EXCLUDED.expires_at, hit_count = intel_cache.hit_count + 1`,
		intelType, value, data, now, now.Add(ttl))
	if serr != nil {
		s.logger.Warn("threatintel: cache store failed (returning fresh result)", "err", serr)
	}
	if notFound {
		return &Response{Type: intelType, Value: value, Result: data, Stale: false, FetchedAt: now, HitCount: 1}, nil
	}
	return &Response{Type: intelType, Value: value, Result: data, Stale: false, FetchedAt: now, HitCount: 1}, nil
}

// refresh 异步回源刷新（懒过期路径——失败仅落日志，下轮查询再试）。
func (s *Service) refresh(ctx context.Context, intelType, value string, ttl time.Duration) {
	if _, err := s.fetchAndStore(ctx, intelType, value, ttl); err != nil {
		s.logger.Warn("threatintel: background refresh failed", "type", intelType, "value", value, "err", err)
	}
}

func (s *Service) ttlOf(intelType string) time.Duration {
	if d, ok := s.ttl[intelType]; ok && d > 0 {
		return d
	}
	return 7 * 24 * time.Hour // 缺省 7 天（保守档）
}

// CleanupExpired 周期物理清理（骑 CronScheduler 低频防表膨胀——A.1）。
// ReentrantJob 契约：幂等（DELETE 语义天然可重入）。
func (s *Service) CleanupExpired(ctx context.Context) error {
	ct, err := s.db.Exec(ctx, `DELETE FROM intel_cache WHERE expires_at < NOW() - interval '7 days'`)
	if err != nil {
		return fmt.Errorf("intel cache cleanup: %w", err)
	}
	if ct.RowsAffected() > 0 {
		s.logger.Info("threatintel: expired rows purged", "rows", ct.RowsAffected())
	}
	return nil
}

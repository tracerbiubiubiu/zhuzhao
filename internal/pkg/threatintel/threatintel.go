// Package threatintel 威胁情报查询端口 + HTTP adapter + 两态桩（S9 B9-1——
// 09 号 §4/附录 A.2：ACL 防腐层，外部模型不渗入域模型）。
//
// 契约未到 = 端口 + 两态桩先行（与 S6 UIP 桩同口径防虚标）：
//   - intel.upstream.stub=true  → 开发态桩（构造样例 + 落日志，全链可测）；
//   - stub=false 且 endpoint 空 → fail-fast ErrIntelNotConfigured。
//
// HTTP adapter 的映射层独立函数 mapUpstreamResponse——真实契约到手只改此函数
// （§24 SSOProvider 同款预留模式）。
package threatintel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// Query 查询请求（type ∈ config 白名单；value 归一化后落缓存键）。
type Query struct {
	Type  string
	Value string
}

// IntelResult 查询结果（拍板点 2：stale 元字段 + fetched_at——消费者自行决定采信度）。
type IntelResult struct {
	Data      json.RawMessage // 上游响应透传（ACL：不解析外部字段）
	FetchedAt time.Time
	Stale     bool
}

// Provider 情报源端口（多实现天然支持——多源聚合属 phase6 R1 查询层）。
type Provider interface {
	Query(ctx context.Context, q Query) (json.RawMessage, error)
}

// Config 上游配置（intel.upstream.*）。
type Config struct {
	Stub     bool   `mapstructure:"stub"`
	Endpoint string `mapstructure:"endpoint"` // 真实契约到手填（stub=false 且空 → fail-fast）
	APIKey   string `mapstructure:"api_key"`  // 鉴权凭据 config 化禁硬编码（README §6 ③）
	Timeout  string `mapstructure:"timeout"`  // 缺省 10s
}

// 错误语义（92600-92602 定义在 errcode 包——本包转发引用保持 errors.Is 可判）。
var (
	ErrIntelNotConfigured = errcode.ErrIntelNotConfigured
	ErrIntelUpstreamQuota = errcode.ErrIntelUpstreamQuota
	ErrIntelUpstream      = errcode.ErrIntelUpstream
	// ErrIntelNotFound 上游 404（查无记录）——正常结果入缓存，非错误（评审批深审项 D）
	ErrIntelNotFound = fmt.Errorf("threatintel: not found")
)

// StubProvider 开发态桩（构造样例——全链可测；返回 200 形态 JSON）。
type StubProvider struct {
	Logger *slog.Logger
}

// Query 桩实现：返回固定样例（带查询回显——测试可断言缓存行归属）。
func (s *StubProvider) Query(_ context.Context, q Query) (json.RawMessage, error) {
	if s.Logger != nil {
		s.Logger.Info("threatintel: stub provider hit", "type", q.Type, "value", q.Value)
	}
	return json.RawMessage(fmt.Sprintf(`{"stub":true,"type":%q,"value":%q,"risk":"low"}`, q.Type, q.Value)), nil
}

// HTTPProvider 通用 HTTP 实现（映射层独立——契约到手只改 mapUpstreamResponse）。
type HTTPProvider struct {
	cfg    Config
	client *http.Client
	logger *slog.Logger
}

// NewProvider 装配：stub → 桩；否则 endpoint 必填（空 → nil + ErrIntelNotConfigured 语义
// 由调用方 fail-fast；此处返回 error 供启动期拒配）。
func NewProvider(cfg Config, logger *slog.Logger) (Provider, error) {
	if logger == nil {
		logger = slog.Default()
	}
	if cfg.Stub {
		return &StubProvider{Logger: logger}, nil
	}
	if cfg.Endpoint == "" {
		return nil, ErrIntelNotConfigured
	}
	timeout, err := time.ParseDuration(cfg.Timeout)
	if err != nil || timeout <= 0 {
		// 非法值降级 10s 并留痕——静默降级与仓内 fail-fast 口径不一致（审计 #12）；
		// 不拒启：timeout 非安全边界，误配只影响上游等待时长
		if logger != nil {
			logger.Warn("intel.upstream.timeout invalid, fallback 10s", "configured", cfg.Timeout, "err", err)
		}
		timeout = 10 * time.Second
	}
	return &HTTPProvider{cfg: cfg, client: &http.Client{Timeout: timeout}, logger: logger}, nil
}

// Query HTTP 实现：GET {endpoint}?type=&value=，鉴权头 Bearer（契约到手随
// mapUpstreamResponse 同点调整）。状态语义：200→透传；404→ErrIntelNotFound
// （正常入缓存）；429→ErrIntelUpstreamQuota；其余→ErrIntelUpstream。
func (p *HTTPProvider) Query(ctx context.Context, q Query) (json.RawMessage, error) {
	u := p.cfg.Endpoint + "?" + url.Values{"type": {q.Type}, "value": {q.Value}}.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, fmt.Errorf("threatintel: build request: %w", err)
	}
	if p.cfg.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+p.cfg.APIKey)
	}
	resp, err := p.client.Do(req)
	if err != nil {
		return nil, ErrIntelUpstream
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB 上限防上游异常大响应
	if err != nil {
		return nil, ErrIntelUpstream
	}
	switch {
	case resp.StatusCode == http.StatusOK:
		return mapUpstreamResponse(body)
	case resp.StatusCode == http.StatusNotFound:
		return nil, ErrIntelNotFound
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, ErrIntelUpstreamQuota
	default:
		p.logger.Warn("threatintel: upstream error", "status", resp.StatusCode)
		return nil, ErrIntelUpstream
	}
}

// mapUpstreamResponse 上游响应 → 缓存 result（ACL 防腐映射层——真实契约到手
// 只改此函数：字段挑选/重命名/404-vs-200+空结果形态对表）。
// 当前实现（占位契约）：响应体原样透传（合法 JSON 校验）。
func mapUpstreamResponse(body []byte) (json.RawMessage, error) {
	if !json.Valid(body) {
		return nil, ErrIntelUpstream
	}
	return json.RawMessage(body), nil
}

// Normalize 归一化（A.1——按类型注册；ip/url 原样，domain/hash 小写 trim）。
func Normalize(intelType, value string) string {
	v := strings.TrimSpace(value)
	switch intelType {
	case "domain", "hash":
		return strings.ToLower(v)
	default: // ip / url / 未来类型
		return v
	}
}

// ValidType config 白名单校验（A.1：intel_type 枚举）。
func ValidType(t string) bool {
	switch t {
	case "ip", "domain", "hash", "url":
		return true
	}
	return false
}

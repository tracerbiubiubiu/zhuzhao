// P2-2–4（2026-11-20 审计续批）定向测试：
// ① provideRateLimitConfig 注入 /api/v1/intel/query 按用户独立桶（intel.query_rate_per_min
//
//	兑现——09 号 A.4「query 端点挂 rate_limit 独立桶：外部配额保护的镜像，双保险」，
//	此前该字段仅有 viper 缺省、Go 运行路径零消费者）；
//
// ② intelTTLFromConfig 非法/非正 TTL 留痕排除（消费端 ttlOf 既定 7 天缺省兜底）。
package app

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

const intelQueryPath = "/api/v1/intel/query"

func TestProvideRateLimitConfig_IntelQueryPerMinuteBucket(t *testing.T) {
	t.Run("default-60-becomes-route-rule-1rps-burst60", func(t *testing.T) {
		cfg := &config.Config{
			RateLimit: config.RateLimitConfig{Enabled: true, Default: config.RateLimitRule{RPS: 20, Burst: 40}},
		}
		cfg.Intel.QueryRatePerMin = 60 // viper 缺省口径
		rl := provideRateLimitConfig(cfg)
		rule, ok := rl.Routes[intelQueryPath]
		require.True(t, ok, "query_rate_per_min=60 应注入 intel 独立桶路由规则")
		require.Equal(t, 1.0, rule.RPS, "每分钟 60 次 = 1 rps")
		require.Equal(t, 60, rule.Burst, "桶容量=整分钟配额")
		require.Equal(t, 20.0, rl.Default.RPS, "默认规则不受注入影响")
	})

	t.Run("sub-minute-precision-rate-30", func(t *testing.T) {
		cfg := &config.Config{RateLimit: config.RateLimitConfig{Enabled: true, Default: config.RateLimitRule{RPS: 20, Burst: 40}}}
		cfg.Intel.QueryRatePerMin = 30
		rule := provideRateLimitConfig(cfg).Routes[intelQueryPath]
		require.Equal(t, 0.5, rule.RPS, "rate<60 时保留 rate/60 亚 1 精度")
		require.Equal(t, 30, rule.Burst)
	})

	t.Run("zero-disables-injection", func(t *testing.T) {
		cfg := &config.Config{RateLimit: config.RateLimitConfig{Enabled: true, Default: config.RateLimitRule{RPS: 20, Burst: 40}}}
		cfg.Intel.QueryRatePerMin = 0 // 0=关（兼容行为：仅走通用默认/覆盖规则）
		rl := provideRateLimitConfig(cfg)
		_, ok := rl.Routes[intelQueryPath]
		require.False(t, ok, "query_rate_per_min=0 不注入 intel 专属桶")
	})

	t.Run("explicit-route-override-wins", func(t *testing.T) {
		userRule := config.RateLimitRule{RPS: 3, Burst: 5}
		cfg := &config.Config{RateLimit: config.RateLimitConfig{
			Enabled: true,
			Default: config.RateLimitRule{RPS: 20, Burst: 40},
			Routes:  map[string]config.RateLimitRule{intelQueryPath: userRule},
		}}
		cfg.Intel.QueryRatePerMin = 60
		rl := provideRateLimitConfig(cfg)
		require.Equal(t, userRule, rl.Routes[intelQueryPath], "用户显式 routes 覆盖优先，不自动改写")
	})

	t.Run("global-rate-limit-disabled-no-injection", func(t *testing.T) {
		cfg := &config.Config{RateLimit: config.RateLimitConfig{Enabled: false, Default: config.RateLimitRule{RPS: 20, Burst: 40}}}
		cfg.Intel.QueryRatePerMin = 60
		rl := provideRateLimitConfig(cfg)
		_, ok := rl.Routes[intelQueryPath]
		require.False(t, ok, "限流中间件整体关闭（直通）时注入无从生效，保持配置干净")
	})

	t.Run("source-config-not-mutated", func(t *testing.T) {
		cfg := &config.Config{RateLimit: config.RateLimitConfig{Enabled: true, Default: config.RateLimitRule{RPS: 20, Burst: 40}}}
		cfg.Intel.QueryRatePerMin = 60
		_ = provideRateLimitConfig(cfg)
		_, ok := cfg.RateLimit.Routes[intelQueryPath]
		require.False(t, ok, "注入写副本，不得污染全局 cfg（Routes map 为引用共享）")
	})
}

func TestIntelTTLFromConfig_SkipsInvalidAndNonPositive(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	got := intelTTLFromConfig(map[string]string{
		"ip": "168h", "hash": "720h",
		"bad": "abc", "neg": "-5h", "zero": "0s",
	}, logger)
	require.Equal(t, 168*time.Hour, got["ip"])
	require.Equal(t, 720*time.Hour, got["hash"])
	for _, k := range []string{"bad", "neg", "zero"} {
		_, ok := got[k]
		require.False(t, ok, "非法/非正 TTL 项 %q 不入表——消费端 ttlOf 既定 7 天缺省生效", k)
	}
}

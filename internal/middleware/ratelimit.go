// API 级限流（07 §2；批次 B 复用）：Redis Lua 令牌桶，user_id（登录后）/
// ClientIP（匿名）双键，路由覆盖规则；超限 429 + Retry-After；
// Redis 不可用 fail-close 503（对齐 Phase 1 登录限流口径）。
package middleware

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// tokenBucketLua 令牌桶：KEYS[1] = 桶键；ARGV = [rps, burst, now_unix_sec]。
// 时间源用应用侧 now（内网 NTP 环境；升级路径 = redis TIME）。
// 返回 [allowed(0/1), retry_after_sec]。
const tokenBucketLua = `
local key = KEYS[1]
local rps = tonumber(ARGV[1])
local burst = tonumber(ARGV[2])
local now = tonumber(ARGV[3])
local b = redis.call('HMGET', key, 'tokens', 'ts')
local tokens = tonumber(b[1])
local ts = tonumber(b[2])
if tokens == nil then tokens = burst end
if ts == nil then ts = now end
local elapsed = now - ts
if elapsed < 0 then elapsed = 0 end
tokens = math.min(burst, tokens + elapsed * rps)
local allowed = 0
local retry = 0
if tokens >= 1 then
  tokens = tokens - 1
  allowed = 1
else
  retry = math.max(1, math.ceil((1 - tokens) / rps))
end
redis.call('HMSET', key, 'tokens', tokens, 'ts', now)
redis.call('EXPIRE', key, math.max(1, math.ceil(burst / rps)) + 5)
return {allowed, retry}
`

// RateLimit 全 API 限流（挂 v1 组：登录后按 user_id、匿名按 ClientIP）。
// routes 精确匹配 path 覆盖 default 规则；Enabled=false 或 rdb 为 nil 直接放行
// （测试构造）；Redis 错误 fail-close → 503 + 10008（对齐 Phase 1 登录限流）。
func RateLimit(rdb *goredis.Client, cfg config.RateLimitConfig) gin.HandlerFunc {
	script := goredis.NewScript(tokenBucketLua)
	rule := func(path string) config.RateLimitRule {
		if rule, ok := cfg.Routes[path]; ok && rule.RPS > 0 {
			return rule
		}
		return cfg.Default
	}
	return func(c *gin.Context) {
		if !cfg.Enabled || rdb == nil {
			c.Next()
			return
		}
		dim := c.GetString("userID")
		if dim == "" {
			dim = "ip:" + c.ClientIP()
		} else {
			dim = "u:" + dim
		}
		r := rule(c.Request.URL.Path)
		if r.RPS <= 0 || r.Burst <= 0 {
			c.Next()
			return
		}
		key := "rl:" + dim
		res, err := script.Run(c.Request.Context(), rdb, []string{key},
			r.RPS, r.Burst, time.Now().Unix()).Result()
		if err != nil {
			response.Fail(c, http.StatusServiceUnavailable,
				errcode.ErrServiceUnavailable.Code, "限流器暂不可用，请稍后重试")
			c.Abort()
			return
		}
		vals, _ := res.([]any)
		allowed, _ := vals[0].(int64)
		if allowed == 1 {
			c.Next()
			return
		}
		retry, _ := vals[1].(int64)
		if retry < 1 {
			retry = 1
		}
		c.Header("Retry-After", strconv.FormatInt(retry, 10))
		c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
			"code":       errcode.ErrTooManyReqs.Code,
			"message":    errcode.ErrTooManyReqs.Message,
			"data":       nil,
			"request_id": c.GetString("request_id"),
		})
	}
}

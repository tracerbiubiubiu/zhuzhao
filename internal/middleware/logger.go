package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/reqid"
)

// RequestID 生成或传递 trace ID，串联 slog 日志。
// D2-24：客户端传入的 X-Request-ID 校验格式（req- + 32 位小写 hex）——
// 原无条件信任，恶意任意串可污染日志/追踪关联且无长度上限（日志膨胀）
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if !isValidRequestID(rid) {
			rid = generateRequestID()
		}
		c.Set("request_id", rid)
		c.Header("X-Request-ID", rid)
		// 注入 request context：service/repo 层（handler 只传 c.Request.Context()）
		// 即可读取（reqid.From），全链路（判定日志/事件/审计）同键
		c.Request = c.Request.WithContext(reqid.With(c.Request.Context(), rid))
		c.Next()
	}
}

// operatorOf 取操作者：JWT 用户（ctx username）优先，服务间验签请求取 utils
// GinMiddleware 透传的 X-Operator（2026-09-16 验签统一批写入），匿名兜底。
// fallback 保持 anonymous（=「无身份的人类请求」），不与生态 system（服务动作）混义。
func operatorOf(c *gin.Context) string {
	if u := c.GetString("username"); u != "" {
		return u
	}
	if op := c.GetString(aksk.ContextKeyOperator); op != "" {
		return op
	}
	return "anonymous"
}

// authOf 身份平面：jwt（用户路由）/ aksk（内网回调验签）/ none（匿名）。
// 与 operator 分列——「谁在操作」与「以什么形式接入」是两个维度，合并必有损；
// M-SSO 上线后可平滑细分为 jwt:local/jwt:sso（2026-09-16 归因口径拍板，选项 4）。
func authOf(c *gin.Context) string {
	if c.GetString("username") != "" {
		return "jwt"
	}
	if c.GetString(aksk.ContextKeyCaller) != "" {
		return "aksk"
	}
	return "none"
}

// truncStr 截断超长字符串（对齐基线"参数 4KB 截断"口径）。
func truncStr(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

// isValidRequestID 本服务生成的 request_id 格式（req-{32 hex}）
func isValidRequestID(rid string) bool {
	if len(rid) != 4+32 || rid[:4] != "req-" {
		return false
	}
	for _, ch := range rid[4:] {
		if !(ch >= '0' && ch <= '9' || ch >= 'a' && ch <= 'f') {
			return false
		}
	}
	return true
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "req-" + hex.EncodeToString(b)
}

// AccessLogger 请求日志中间件（access log）。
// B4-6：跳过健康检查路径——K8s 探针数秒一次，避免日志噪音稀释有效请求
// （09-middleware.md 伪代码的 WithSkipPath 语义）
func AccessLogger(logger *slog.Logger) gin.HandlerFunc {
	skipPaths := map[string]struct{}{
		"/health/live":  {},
		"/health/ready": {},
	}
	return func(c *gin.Context) {
		if _, skip := skipPaths[c.Request.URL.Path]; skip {
			c.Next()
			return
		}
		start := time.Now()
		c.Next()

		// 行名/字段名对齐 standards §6 访问日志标准字段（duration_ms 统一口径，
		// 对齐 activelist/taskrunner；auth/caller 见 operatorOf/authOf 注释）。
		// caller 恒出（无验签场景为空串）——三仓行结构稳定，ES 索引友好
		// （2026-09-16 与 taskrunner/activelist 对齐后的统一惯例）
		attrs := []slog.Attr{
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.String("query", truncStr(c.Request.URL.RawQuery, 4096)),
			slog.String("operator", operatorOf(c)),
			slog.String("auth", authOf(c)),
			slog.Int("status", c.Writer.Status()),
			slog.Int("size", c.Writer.Size()),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
			slog.String("ip", c.ClientIP()),
			slog.String("request_id", c.GetString("request_id")),
			slog.String("caller", c.GetString(aksk.ContextKeyCaller)),
		}
		logger.LogAttrs(c.Request.Context(), slog.LevelInfo, "access", attrs...)
	}
}

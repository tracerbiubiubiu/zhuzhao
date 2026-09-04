package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"

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

		logger.Info("request",
			slog.String("method", c.Request.Method),
			slog.String("path", c.Request.URL.Path),
			slog.Int("status", c.Writer.Status()),
			slog.Int("size", c.Writer.Size()),
			slog.Duration("latency", time.Since(start)),
			slog.String("ip", c.ClientIP()),
			slog.String("request_id", c.GetString("request_id")),
		)
	}
}

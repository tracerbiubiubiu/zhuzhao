package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// RequestID 生成或传递 trace ID，串联 slog 日志
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		rid := c.GetHeader("X-Request-ID")
		if rid == "" {
			rid = generateRequestID()
		}
		c.Set("request_id", rid)
		c.Header("X-Request-ID", rid)
		c.Next()
	}
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return "req-" + hex.EncodeToString(b)
}

// AccessLogger 请求日志中间件（access log）。
// B4-6：跳过健康检查路径——K8s 探针数秒一次，避免日志噪音稀释有效请求
//（09-middleware.md 伪代码的 WithSkipPath 语义）
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

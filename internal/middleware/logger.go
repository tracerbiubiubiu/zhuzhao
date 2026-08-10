package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"log/slog"
)

// RequestID 请求 ID 中间件
func RequestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-Id")
		if requestID == "" {
			requestID = uuid.New().String()
		}
		c.Set("request_id", requestID)
		c.Header("X-Request-Id", requestID)
		c.Next()
	}
}

// Logger 请求日志中间件
func Logger(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
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

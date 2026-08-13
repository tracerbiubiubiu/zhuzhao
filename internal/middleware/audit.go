package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"time"

	"github.com/gin-gonic/gin"
)

// AuditLogger 审计日志写入接口（避免直接依赖 repository 包）
type AuditLogger interface {
	Insert(ctx context.Context, log AuditLogEntry) error
}

// AuditLogEntry 审计日志条目
type AuditLogEntry struct {
	UserID      int64
	Username    string
	Method      string
	Path        string
	StatusCode  int
	Duration    int64
	IP          string
	UserAgent   string
	RequestBody string
	CreatedAt   time.Time
}

// AuditLog 操作日志中间件（同步写入 DB）
func AuditLog(auditLogger AuditLogger) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()

		// 读取请求体（用于记录操作参数）
		var bodyBytes []byte
		if c.Request.Body != nil {
			bodyBytes, _ = io.ReadAll(c.Request.Body)
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// 执行请求
		c.Next()

		// 同步记录（响应已返回，用户无感知）
		entry := AuditLogEntry{
			UserID:      c.GetInt64("userID"),
			Username:    c.GetString("username"),
			Method:      c.Request.Method,
			Path:        c.Request.URL.Path,
			StatusCode:  c.Writer.Status(),
			Duration:    time.Since(start).Milliseconds(),
			IP:          c.ClientIP(),
			UserAgent:   c.Request.UserAgent(),
			RequestBody: maskSensitive(bodyBytes),
			CreatedAt:   time.Now(),
		}

		// 同步写入 DB，失败只记应用日志，不影响业务
		if err := auditLogger.Insert(c.Request.Context(), entry); err != nil {
			slog.Error("audit log write failed", "err", err, "path", entry.Path)
		}
	}
}

// maskSensitive 敏感字段脱敏
func maskSensitive(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		return string(body)
	}
	sensitiveKeys := []string{"password", "old_password", "new_password", "secret", "token"}
	for _, key := range sensitiveKeys {
		if _, ok := m[key]; ok {
			m[key] = "***"
		}
	}
	result, _ := json.Marshal(m)
	return string(result)
}

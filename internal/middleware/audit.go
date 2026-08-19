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
			// ReadAll 会耗尽原 Body；用内存副本替换，供后续 handler 再次 BindJSON
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

		// F-5 修复：请求 context 随客户端断连而取消，直接用它写库会丢审计
		// （恰恰是"执行删除后立刻断连规避审计"的攻击场景）。改用
		// WithoutCancel + 独立超时，保证审计写入不随请求终止。
		ctx, cancel := context.WithTimeout(context.WithoutCancel(c.Request.Context()), auditWriteTimeout)
		defer cancel()
		if err := auditLogger.Insert(ctx, entry); err != nil {
			slog.Error("audit log write failed", "err", err, "path", entry.Path)
		}
	}
}

// auditWriteTimeout 审计写入的独立超时（与请求生命周期解耦）
const auditWriteTimeout = 3 * time.Second

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

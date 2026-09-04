package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"strings"
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
	RequestID   string // 03 §3.4：与 slog/判定日志/事件/taskrunner 同键关联
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

		// D2-12：先把响应 flush 给客户端再做同步审计写——net/http 在整个
		// 中间件链返回后才真正 flush，若此处不主动 Flush，客户端要等审计
		// INSERT 完成（最长 auditWriteTimeout）才收到响应，DB 抖动时尾延迟放大
		c.Writer.Flush()

		// 同步记录（响应已送达客户端；Shutdown drain 保证进程内审计仍落库）
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
			RequestID:   c.GetString("request_id"),
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

// maxAuditBody 审计请求体入库上限（D2-08）——登录失败等未认证路径同步写
// audit_logs，request_body 无截断上限时可单行灌入超大内容（TEXT 列慢性膨胀 DoS）
const maxAuditBody = 2048

// maskSensitive 敏感字段脱敏（D2-19：递归 + 大小写不敏感 + 非法 JSON 占位）。
// 原实现仅顶层精确匹配：嵌套对象/数组内的 password 不脱敏、Password 大小写
// 绕过、Unmarshal 失败原文入库（form-encoded 等非 JSON body 整段落库）
func maskSensitive(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(body, &m) != nil {
		// D2-19：非法 JSON 不原文入库——占位记录长度，保留可观测性不放大存储
		return fmt.Sprintf("<binary len=%d>", len(body))
	}
	maskSensitiveMap(m)
	result, _ := json.Marshal(m)
	return truncateAuditBody(string(result))
}

// maskSensitiveMap 递归脱敏 map（含嵌套对象与数组）
func maskSensitiveMap(m map[string]any) {
	sensitiveKeys := []string{"password", "old_password", "new_password", "secret", "token"}
	for key, val := range m {
		for _, sk := range sensitiveKeys {
			if strings.EqualFold(key, sk) {
				m[key] = "***"
				break
			}
		}
		if val == nil {
			continue
		}
		switch typed := val.(type) {
		case map[string]any:
			maskSensitiveMap(typed)
		case []any:
			for _, item := range typed {
				if child, ok := item.(map[string]any); ok {
					maskSensitiveMap(child)
				}
			}
		}
	}
}

// truncateAuditBody 截断超限审计体（D2-08）
func truncateAuditBody(s string) string {
	if len(s) <= maxAuditBody {
		return s
	}
	return s[:maxAuditBody] + fmt.Sprintf(`...<truncated, total=%d>`, len(s))
}

package middleware

import (
	"log/slog"
	"runtime/debug"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/response"
)

// Recovery Panic 恢复中间件
func Recovery(logger *slog.Logger) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				logger.Error("panic recovered",
					slog.Any("error", err),
					slog.String("stack", string(debug.Stack())),
					slog.String("path", c.Request.URL.Path),
				)
				response.InternalError(c, "服务器内部错误")
				c.Abort()
				return
			}
		}()
		c.Next()
	}
}

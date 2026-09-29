package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
)

// PanicSink P4-8 panic 落库出口（最小接口——PanicRepo 实现；nil=只记日志不落库）
type PanicSink interface {
	Upsert(ctx context.Context, fingerprint, message, stack, path string) error
}

// Recovery Panic 恢复中间件（sink 非空时聚合落库——异步尽力而为，绝不阻塞恢复路径）
func Recovery(logger *slog.Logger, sink PanicSink) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				stack := string(debug.Stack())
				logger.Error("panic recovered",
					slog.Any("error", err),
					slog.String("stack", stack),
					slog.String("path", c.Request.URL.Path),
				)
				IncPanic() // P4-8 指标计数（与落库同位）
				if sink != nil {
					go func(msg, stack, path string) {
						// 脱离请求取消链（panic 后请求 ctx 不可靠）；有限时长防挂起
						ctx, cancel := context.WithTimeout(context.WithoutCancel(context.Background()), 3*time.Second)
						defer cancel()
						// 指纹=message+栈首 3 行（goroutine 头行含地址不稳定，取后续帧）
						lines := strings.Split(stack, "\n")[1:] // 审计修复：跳过首行 goroutine N 头行（N 易变致同因 panic 聚合失效）
						if len(lines) > 6 {
							lines = lines[:6]
						}
						h := sha256.Sum256([]byte(msg + strings.Join(lines, "\n")))
						_ = sink.Upsert(ctx, hex.EncodeToString(h[:]), msg, stack, path)
					}(fmt.Sprint(err), stack, c.Request.URL.Path)
				}
				response.InternalError(c, "服务器内部错误")
				c.Abort()
				return
			}
		}()
		c.Next()
	}
}

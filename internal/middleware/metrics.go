package middleware

import (
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	"github.com/gin-gonic/gin"
)

// P4-8 /metrics 指标（Prometheus 文本格式——手写零依赖；进程内原子计数器）。
// 住 middleware 包：埋点与渲染同址，handler 只挂路由（防 handler↔middleware 环）。

var (
	metricRequests atomic.Int64
	metricPanics   atomic.Int64
	metric5xx      atomic.Int64
	metricStart    = time.Now()
)

// MetricsMiddleware 计数埋点（全局链——AccessLogger 同位）
func MetricsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		metricRequests.Add(1)
		if c.Writer.Status() >= 500 {
			metric5xx.Add(1)
		}
	}
}

// IncPanic panic 计数（Recovery 落库同位调用）
func IncPanic() { metricPanics.Add(1) }

// RenderMetrics Prometheus 文本渲染（GET /metrics——根级非 /api/v1，catalog 天然豁免）
func RenderMetrics(c *gin.Context) {
	var b strings.Builder
	b.WriteString("# TYPE zhuzhao_requests_total counter\n")
	fmt.Fprintf(&b, "zhuzhao_requests_total %d\n", metricRequests.Load())
	b.WriteString("# TYPE zhuzhao_panics_total counter\n")
	fmt.Fprintf(&b, "zhuzhao_panics_total %d\n", metricPanics.Load())
	b.WriteString("# TYPE zhuzhao_5xx_total counter\n")
	fmt.Fprintf(&b, "zhuzhao_5xx_total %d\n", metric5xx.Load())
	b.WriteString("# TYPE zhuzhao_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "zhuzhao_uptime_seconds %.0f\n", time.Since(metricStart).Seconds())
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(b.String()))
}

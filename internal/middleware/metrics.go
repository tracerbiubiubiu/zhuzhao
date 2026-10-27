package middleware

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
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

// 带标号业务水位指标（2026-10-27 A-2 拍板 C-6/C-16 观测批——storage 用量按
// purpose / notify 发送量 ok·dead / GC 回收量按 purpose）。与上方三计数器同址
// 同渲染（P4-8「埋点与渲染同址」口径延续）；label 值域受控（purpose/结果枚举），
// 渲染不转义。
var (
	metricLabeledMu       sync.RWMutex
	metricLabeledGauges   = map[string]map[string]float64{}
	metricLabeledCounters = map[string]map[string]float64{}
)

// SetGauge 设置带标号 gauge 当前值（采样器全量覆盖——SQL 聚合语义）
func SetGauge(name, label string, v float64) {
	metricLabeledMu.Lock()
	defer metricLabeledMu.Unlock()
	if metricLabeledGauges[name] == nil {
		metricLabeledGauges[name] = map[string]float64{}
	}
	metricLabeledGauges[name][label] = v
}

// AddCounter 带标号计数器累加（单调递增——读取方算差值）
func AddCounter(name, label string, delta float64) {
	metricLabeledMu.Lock()
	defer metricLabeledMu.Unlock()
	if metricLabeledCounters[name] == nil {
		metricLabeledCounters[name] = map[string]float64{}
	}
	metricLabeledCounters[name][label] += delta
}

// GetCounter 读带标号计数器当前值（量级 cron 算小时差——进程内单实例口径）
func GetCounter(name, label string) float64 {
	metricLabeledMu.RLock()
	defer metricLabeledMu.RUnlock()
	return metricLabeledCounters[name][label]
}

// renderLabeled 按指标名稳定排序渲染（name{label="..."} value——Prometheus 文本格式）
func renderLabeled(b *strings.Builder, typ string, m map[string]map[string]float64) {
	names := make([]string, 0, len(m))
	for n := range m {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(b, "# TYPE %s %s\n", n, typ)
		labels := make([]string, 0, len(m[n]))
		for l := range m[n] {
			labels = append(labels, l)
		}
		sort.Strings(labels)
		for _, l := range labels {
			fmt.Fprintf(b, "%s{label=%q} %g\n", n, l, m[n][l])
		}
	}
}

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
	metricLabeledMu.RLock()
	renderLabeled(&b, "counter", metricLabeledCounters)
	renderLabeled(&b, "gauge", metricLabeledGauges)
	metricLabeledMu.RUnlock()
	c.Data(http.StatusOK, "text/plain; version=0.0.4; charset=utf-8", []byte(b.String()))
}

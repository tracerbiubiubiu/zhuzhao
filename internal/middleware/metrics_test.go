package middleware

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// C-6/C-16（2026-10-27 观测批）：带标号 gauge/counter 的写入与渲染契约。
// 渲染必须产出合法 Prometheus 文本（TYPE 行+带标号样本行）——未来接抓取端零改造。
func TestLabeledMetrics_Render(t *testing.T) {
	gin.SetMode(gin.TestMode)
	SetGauge("zhuzhao_storage_usage_bytes", "avatar", 1024)
	SetGauge("zhuzhao_storage_usage_bytes", "ticket_richtext", 2048)
	SetGauge("zhuzhao_storage_usage_bytes", "total", 3072)
	AddCounter("zhuzhao_notify_sent_total", "ok", 3)
	AddCounter("zhuzhao_notify_sent_total", "ok", 1) // 累加语义
	AddCounter("zhuzhao_notify_sent_total", "dead", 2)

	if got := GetCounter("zhuzhao_notify_sent_total", "ok"); got != 4 {
		t.Fatalf("ok counter want 4 got %v", got)
	}
	if got := GetCounter("zhuzhao_notify_sent_total", "missing"); got != 0 {
		t.Fatalf("missing label counter want 0 got %v", got)
	}

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	RenderMetrics(c)
	body := w.Body.String()
	for _, want := range []string{
		"# TYPE zhuzhao_storage_usage_bytes gauge\n",
		`zhuzhao_storage_usage_bytes{label="avatar"} 1024`,
		`zhuzhao_storage_usage_bytes{label="total"} 3072`,
		"# TYPE zhuzhao_notify_sent_total counter\n",
		`zhuzhao_notify_sent_total{label="ok"} 4`,
		`zhuzhao_notify_sent_total{label="dead"} 2`,
		"zhuzhao_requests_total ", // 既有三计数器不受扩展影响
	} {
		if !strings.Contains(body, want) {
			t.Errorf("rendered body missing %q\nbody:\n%s", want, body)
		}
	}
}

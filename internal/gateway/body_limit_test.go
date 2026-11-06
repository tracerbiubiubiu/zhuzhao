package gateway

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

// The accepted gateway limit remains 1MiB; its local rejection must not claim
// an upstream outage. Cover chunked requests as well as known Content-Length.
func TestGatewayBodyLimit_Reports413WithoutForwarding(t *testing.T) {
	var calls atomic.Int64
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		body, err := io.ReadAll(req.Body)
		if err != nil {
			t.Errorf("upstream body read: %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if err := (&aksk.Verifier{Keys: map[string][]byte{"zhuzhao": []byte("synthetic-sk")}}).Verify(req, body); err != nil {
			t.Errorf("upstream signature: %v", err)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer up.Close()
	r, err := New([]Upstream{{Prefix: "/al", Target: up.URL, StripPrefix: true}}, "zhuzhao", "synthetic-sk")
	require.NoError(t, err)
	engine := gin.New()
	engine.Use(func(c *gin.Context) {
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 1<<20)
		c.Set("request_id", "body-boundary")
		c.Next()
	})
	r.Mount(engine.Group(""))
	for _, known := range []bool{true, false} {
		for _, size := range []int{1 << 20, (1 << 20) + 1} {
			req := httptest.NewRequest(http.MethodPost, "/al/api/v1/data/synthetic/import", strings.NewReader(strings.Repeat("x", size)))
			if !known {
				req.ContentLength = -1
			}
			before := calls.Load()
			response := serve(engine, req)
			if size == 1<<20 {
				require.Equal(t, http.StatusOK, response.Code)
				require.Equal(t, before+1, calls.Load())
			} else {
				require.Equal(t, http.StatusRequestEntityTooLarge, response.Code, "local body limit is a client error, not upstream failure")
				require.Equal(t, before, calls.Load(), "oversized input must not reach upstream")
				require.Contains(t, response.Body.String(), `"code":10001`)
				require.Contains(t, response.Body.String(), `"request_id":"body-boundary"`)
				require.NotContains(t, response.Body.String(), "上游服务暂时不可用")
			}
		}
	}
}

package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk/compat"
)

// Echo 预接线验证靶（runbook §5.2 判定表）：正向上游协议签名 → 200 + caller
// 归因；坏签名 → 401。签名用 utils compat.Sign 现签（协议核已由 utils 冻结
// 向量 + 双向交叉验证背书，此处只验装配与归因链路）。
func TestCompatEcho(t *testing.T) {
	const (
		ak = "LEGACY_AK_TEST"
		sk = "legacy-sk-test-000000000000"
	)
	gin.SetMode(gin.TestMode)
	r := gin.New()
	v := &compat.Verifier{Keys: map[string][]byte{ak: []byte(sk)}}
	r.POST("/compat/echo", compat.GinMiddleware(v, nil), (&CompatHandler{}).Echo)

	t.Run("valid signature → 200 + caller 归因", func(t *testing.T) {
		body := []byte(`{"ping":"1"}`)
		req := httptest.NewRequest(http.MethodPost, "/compat/echo", bytes.NewReader(body))
		compat.Sign(req, body, compat.SignOptions{AK: ak, SK: []byte(sk)})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
		}
		var env struct {
			Data struct {
				Caller string `json:"caller"`
			} `json:"data"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if env.Data.Caller != ak {
			t.Fatalf("caller = %q, want %q", env.Data.Caller, ak)
		}
	})

	t.Run("bad signature → 401", func(t *testing.T) {
		body := []byte(`{"ping":"1"}`)
		req := httptest.NewRequest(http.MethodPost, "/compat/echo", bytes.NewReader(body))
		compat.Sign(req, body, compat.SignOptions{AK: ak, SK: []byte("wrong-sk")})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401, body = %s", w.Code, w.Body.String())
		}
	})
}

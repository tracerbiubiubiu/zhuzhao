package router

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk/compat"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
)

// compat 预接线装配层测试（2026-10-14）：实弹抓包验证延后（内网调用场景未
// 就位），装配层先在测试内证到——开关门控 / 验签链路 / caller 归因。协议核
// 本身的冻结向量与双向交叉验证在 utils aksk/compat 测试里，此处不重复。
func TestMountCompatGroup(t *testing.T) {
	const (
		ak = "LEGACY_AK_MOUNT"
		sk = "legacy-sk-mount-0000000000"
	)
	gin.SetMode(gin.TestMode)

	build := func(cfg config.CompatAKSKConfig) *gin.Engine {
		r := gin.New()
		mountCompatGroup(r, cfg)
		return r
	}

	t.Run("disabled 默认不挂载 → 404", func(t *testing.T) {
		r := build(config.CompatAKSKConfig{Enabled: false, Keys: map[string]string{ak: sk}})
		req := httptest.NewRequest(http.MethodPost, "/compat/echo", bytes.NewReader([]byte(`{}`)))
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Fatalf("disabled 应无路由：status = %d, want 404", w.Code)
		}
	})

	t.Run("enabled 全链路：签名 → 200 + caller 归因", func(t *testing.T) {
		r := build(config.CompatAKSKConfig{Enabled: true, Keys: map[string]string{ak: sk}})
		body := []byte(`{"probe":"mount"}`)
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
			t.Fatalf("caller = %q, want %q（归因链路：config 密钥环 → 验签器 → gin context → handler）", env.Data.Caller, ak)
		}
	})

	t.Run("enabled 但密钥错 → 401（fail-closed 不放行）", func(t *testing.T) {
		r := build(config.CompatAKSKConfig{Enabled: true, Keys: map[string]string{ak: sk}})
		body := []byte(`{"probe":"mount"}`)
		req := httptest.NewRequest(http.MethodPost, "/compat/echo", bytes.NewReader(body))
		compat.Sign(req, body, compat.SignOptions{AK: ak, SK: []byte("other-sk")})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})

	t.Run("enabled 未知 AK → 401", func(t *testing.T) {
		r := build(config.CompatAKSKConfig{Enabled: true, Keys: map[string]string{ak: sk}})
		body := []byte(`{}`)
		req := httptest.NewRequest(http.MethodPost, "/compat/echo", bytes.NewReader(body))
		compat.Sign(req, body, compat.SignOptions{AK: "unknown-ak", SK: []byte(sk)})
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want 401", w.Code)
		}
	})
}

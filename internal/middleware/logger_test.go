package middleware

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// D2-24 守护：客户端透传 X-Request-ID 的格式校验（req-{32 位小写 hex}）——
// 非法格式一律重新生成，防日志膨胀/追踪污染
func TestIsValidRequestID(t *testing.T) {
	assertValid := []string{
		"req-550e8400e29b41d4a716446655440000",
		"req-00000000000000000000000000000000",
		"req-ffffffffffffffffffffffffffffffff",
	}
	for _, s := range assertValid {
		if !isValidRequestID(s) {
			t.Errorf("isValidRequestID(%q) = false, want true", s)
		}
	}

	assertInvalid := []string{
		"",                                            // 空
		"req-",                                        // 缺 hex 段
		"req-550e8400e29b41d4a71644665544000",         // 31 位
		"req-550e8400e29b41d4a7164466554400000",       // 33 位
		"req-550E8400E29B41D4A716446655440000",        // 大写 hex
		"req-550e8400e29b41d4a71644665544gggg",        // 非 hex 字符
		"550e8400e29b41d4a716446655440000",            // 缺前缀
		"trace-550e8400e29b41d4a716446655440000",      // 错前缀
		"req-550e8400e29b41d4a716446655440000\nx:1",   // 注入形态（logfmt 污染）
		"req-" + string(make([]byte, 32)) + "padding", // 超长
	}
	for _, s := range assertInvalid {
		if isValidRequestID(s) {
			t.Errorf("isValidRequestID(%q) = true, want false", s)
		}
	}
}

// 归因口径（2026-09-16 选项 4）：operator 与 auth 分列——三个身份平面各得其所，
// caller 仅 aksk 场景出字段。
func TestAccessLogAttribution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	build := func(setCtx func(c *gin.Context)) (*gin.Context, *bytes.Buffer) {
		c, _ := gin.CreateTestContext(httptest.NewRecorder())
		c.Request = httptest.NewRequest(http.MethodPost, "/x", nil)
		if setCtx != nil {
			setCtx(c)
		}
		return c, nil
	}

	cases := []struct {
		name         string
		setCtx       func(c *gin.Context)
		wantOperator string
		wantAuth     string
	}{
		{"jwt 用户路由", func(c *gin.Context) { c.Set("username", "E000001") }, "E000001", "jwt"},
		{"aksk 回调带 X-Operator", func(c *gin.Context) {
			c.Set("caller", "taskrunner")
			c.Set("operator", "admin")
		}, "admin", "aksk"},
		{"aksk 回调缺 X-Operator", func(c *gin.Context) {
			c.Set("caller", "taskrunner")
			c.Set("operator", "system") // utils GinMiddleware 写入的兜底值
		}, "system", "aksk"},
		{"匿名", nil, "anonymous", "none"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c, _ := build(tc.setCtx)
			if got := operatorOf(c); got != tc.wantOperator {
				t.Fatalf("operatorOf = %q, want %q", got, tc.wantOperator)
			}
			if got := authOf(c); got != tc.wantAuth {
				t.Fatalf("authOf = %q, want %q", got, tc.wantAuth)
			}
		})
	}

	// 端到端：jwt 行无 caller 字段，aksk 行有
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, nil))
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(AccessLogger(logger))
	r.POST("/ping", func(c *gin.Context) { c.Set("username", "u1"); c.Status(200) })
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/ping", nil))
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("log line: %v (%s)", err, buf.String())
	}
	if line["auth"] != "jwt" || line["operator"] != "u1" {
		t.Fatalf("fields: %v", line)
	}
	if _, ok := line["caller"]; ok {
		t.Fatalf("jwt 行不应出现 caller 字段: %v", line)
	}
}

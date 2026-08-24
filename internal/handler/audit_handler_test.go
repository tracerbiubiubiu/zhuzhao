package handler

import (
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// D2-45：extractBearer——Authorization 头解析契约（大小写前缀/空值/无前缀）
func TestExtractBearer(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"Bearer abc.def.ghi", "abc.def.ghi"},
		{"bearer abc", ""},         // 前缀须精确大写
		{"Bearer ", ""},            // 空值（len 不大于前缀+1）
		{"Bearer", ""},             // 无空格
		{"Basic dXNlcjpwYXNz", ""}, // 非 Bearer
		{"", ""},
	}
	for _, c := range cases {
		if got := extractBearer(c.in); got != c.want {
			t.Errorf("extractBearer(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// D2-45：审计 start>end / 日期格式 / user_id 非法的参数校验路径
// （B4-6 加入的 400 分支从未有测试——400 在触达 service 前返回，nil service 即可测）
func TestAuditListLogs_ParamValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewAuditHandler(nil) // 校验失败路径不触达 service

	tests := []struct {
		name    string
		query   string
		wantCtx int
	}{
		{"start 晚于 end", "start=2026-01-02&end=2026-01-01", 400},
		{"start 相等 end（合法边界）", "start=2026-01-01&end=2026-01-01", 200}, // 会触达 service → 单独处理
		{"非法日期格式", "start=2026/01/01", 400},
		{"user_id 非数字", "user_id=abc", 400},
	}
	for _, tt := range tests {
		if tt.wantCtx == 200 {
			continue // 合法路径需 DB，属集成测试范围
		}
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/v1/audit/logs?"+tt.query, nil)
			h.ListLogs(c)
			if w.Code != tt.wantCtx {
				t.Fatalf("HTTP = %d, want %d", w.Code, tt.wantCtx)
			}
		})
	}
}

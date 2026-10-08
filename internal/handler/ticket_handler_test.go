package handler

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

// 工单列表新增筛选（keyword/created_from/created_to/created_by）的 400 校验路径——
// 校验失败在触达 service 前返回，nil service 即可测（对齐审计列表 B4-6 测试法）
func TestTicketList_ParamValidation(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewTicketHandler(nil)

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"keyword 超长（51 字符）", "keyword=" + strings.Repeat("a", 51), 400},
		{"created_from 非法格式", "created_from=2026/01/01", 400},
		{"created_to 非法格式", "created_to=01-01-2026", 400},
		{"created_from 晚于 created_to（写反）", "created_from=2026-01-03&created_to=2026-01-01", 400},
		{"created_from 等于 created_to 上界（相邻日写反）", "created_from=2026-01-02&created_to=2026-01-01", 400},
		{"created_by 非 me 字面量", "created_by=123", 400},
		{"assignee 非 me 字面量（存量契约回归）", "assignee=456", 400},
		{"priority 非整数（存量契约回归）", "priority=high", 400},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/api/v1/tickets?"+tt.query, nil)
			h.List(c)
			if w.Code != tt.want {
				t.Fatalf("HTTP = %d, want %d", w.Code, tt.want)
			}
		})
	}
}

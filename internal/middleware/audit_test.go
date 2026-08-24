package middleware

import (
	"encoding/json"
	"strings"
	"testing"
)

// D2-45/D2-19 守护：maskSensitive 递归脱敏 + 大小写不敏感。
// 修复前仅顶层精确匹配：嵌套/数组内 password 裸入库、Password 大小写绕过。
func TestMaskSensitive(t *testing.T) {
	tests := []struct {
		name  string
		input string
		check func(t *testing.T, out string)
	}{
		{
			name:  "顶层脱敏",
			input: `{"employee_no":"E001","password":"secret123"}`,
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "secret123") {
					t.Fatal("顶层 password 未脱敏")
				}
			},
		},
		{
			name:  "大小写不敏感",
			input: `{"Password":"secret123"}`,
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "secret123") {
					t.Fatal("Password（大写）绕过脱敏")
				}
			},
		},
		{
			name:  "嵌套对象递归",
			input: `{"user":{"old_password":"secret123"},"note":"x"}`,
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "secret123") {
					t.Fatal("嵌套 old_password 未脱敏")
				}
			},
		},
		{
			name:  "数组内对象递归",
			input: `{"items":[{"new_password":"secret123"},{"k":"v"}]}`,
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "secret123") {
					t.Fatal("数组内 new_password 未脱敏")
				}
			},
		},
		{
			name:  "token/secret 同样脱敏",
			input: `{"token":"tk-1","secret":"sk-1","safe":"keep"}`,
			check: func(t *testing.T, out string) {
				if strings.Contains(out, "tk-1") || strings.Contains(out, "sk-1") {
					t.Fatal("token/secret 未脱敏")
				}
				if !strings.Contains(out, "keep") {
					t.Fatal("非敏感字段不应被误伤")
				}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out := maskSensitive([]byte(tt.input))
			// 脱敏输出仍应是合法 JSON（入库可解析）
			var m map[string]any
			if err := json.Unmarshal([]byte(out), &m); err != nil {
				t.Fatalf("输出非合法 JSON: %v (%s)", err, out)
			}
			tt.check(t, out)
		})
	}
}

// D2-19/D2-08：非法 JSON 不原文入库（占位记长度）+ 超限截断
func TestMaskSensitive_NonJSONAndTruncate(t *testing.T) {
	// form-encoded 等非 JSON body → 占位（原原文整段落库）
	out := maskSensitive([]byte("password=abc123&user=bob"))
	if strings.Contains(out, "abc123") || strings.Contains(out, "password=") {
		t.Fatalf("非 JSON 原文入库: %s", out)
	}
	if !strings.HasPrefix(out, "<binary len=") {
		t.Fatalf("应落占位标记，实际: %s", out)
	}

	// 超长 JSON → 截断（D2-08，maxAuditBody=2048）
	big := `{"k":"` + strings.Repeat("x", maxAuditBody*2) + `"}`
	out = maskSensitive([]byte(big))
	if len(out) > maxAuditBody+100 {
		t.Fatalf("未截断：len=%d", len(out))
	}
	if !strings.Contains(out, "<truncated") {
		t.Fatalf("截断缺标记: %s", out[len(out)-60:])
	}

	// 空 body
	if out := maskSensitive(nil); out != "" {
		t.Fatalf("空 body 应返回空串，实际 %q", out)
	}
}

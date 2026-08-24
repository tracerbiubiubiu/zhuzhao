package middleware

import "testing"

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

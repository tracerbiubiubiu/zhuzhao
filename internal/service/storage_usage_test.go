package service

import "testing"

// usageLevel 双向档位（C-6 观测批）——E2E 受 60s 节流窗限制，纯函数面覆盖回落沿。
func TestUsageLevel(t *testing.T) {
	cases := []struct {
		name       string
		total, cap int64
		want       int
	}{
		{"容量未配恒 0", 999, 0, 0},
		{"负容量同未配", 10, -1, 0},
		{"零用量", 0, 1000, 0},
		{"79.9% 正常档", 799, 1000, 0},
		{"80% 恰入 warn", 800, 1000, 1},
		{"89.9% warn", 899, 1000, 1},
		{"90% 恰入 critical", 900, 1000, 2},
		{"超容量 critical", 1500, 1000, 2},
		{"回落沿 79% 归零（双向）", 790, 1000, 0},
	}
	for _, c := range cases {
		if got := usageLevel(c.total, c.cap); got != c.want {
			t.Errorf("%s: usageLevel(%d,%d)=%d want %d", c.name, c.total, c.cap, got, c.want)
		}
	}
}

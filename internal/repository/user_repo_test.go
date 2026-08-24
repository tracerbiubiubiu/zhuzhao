package repository

import "testing"

// D2-21 守护：ILIKE 模式元字符转义（配合 ESCAPE '\'）
func TestEscapeLike(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"plain", "plain"},
		{"100%", `100\%`},
		{"a_b", `a\_b`},
		{`back\slash`, `back\\slash`},
		{`%_\`, `\%\_\\`}, // 转义顺序：先反斜杠，再 % _
		{"中文ok", "中文ok"},
	}
	for _, c := range cases {
		if got := escapeLike(c.in); got != c.want {
			t.Errorf("escapeLike(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

// D2-13 守护：page 上限 10000（防 (page-1)*pageSize 溢出回绕负 OFFSET → SQL 500）
func TestNormalizePage(t *testing.T) {
	cases := []struct {
		page, pageSize         int
		wantPage, wantPageSize int
	}{
		{0, 0, 1, 20},              // 默认值
		{-5, -1, 1, 20},            // 负数
		{1, 200, 1, 100},           // pageSize 上限
		{999999999, 20, 10000, 20}, // page 溢出回绕防护
		{10001, 50, 10000, 50},
		{10000, 100, 10000, 100}, // 边界值本身放行
	}
	for _, c := range cases {
		gotPage, gotSize := normalizePage(c.page, c.pageSize)
		if gotPage != c.wantPage || gotSize != c.wantPageSize {
			t.Errorf("normalizePage(%d, %d) = (%d, %d), want (%d, %d)",
				c.page, c.pageSize, gotPage, gotSize, c.wantPage, c.wantPageSize)
		}
	}
}

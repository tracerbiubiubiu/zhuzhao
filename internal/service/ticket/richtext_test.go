package ticket

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// richtext 渲染/消毒 SSOT（05 号 §6）——安全负向为主（11 号 B5-4 验收要点：
// 存储型 XSS 负向用例必测）。
func TestRenderRichtext(t *testing.T) {
	t.Run("标题/列表/代码块/表格/图片渲染", func(t *testing.T) {
		md := "# 标题\n\n- 项一\n- 项二\n\n```go\nprint()\n```\n\n" +
			"| A | B |\n|---|---|\n| 1 | 2 |\n\n![img](/api/v1/files/tickets/2026/10/x.png)"
		html, err := RenderRichtext(md)
		require.NoError(t, err)
		for _, want := range []string{"<h1>", "<li>", "<code>", "<table>", "<img", "/api/v1/files/tickets/2026/10/x.png"} {
			assert.Contains(t, html, want, "排版诉求（05 §6）应保留：%s", want)
		}
	})

	t.Run("MD 内嵌原始 HTML 整体丢弃（goldmark 默认档 raw HTML omitted + bluemonday 清注释）", func(t *testing.T) {
		html, err := RenderRichtext("正文\n\n<script>alert(1)</script>\n\n<b onclick=x>bold</b>")
		require.NoError(t, err)
		assert.NotContains(t, html, "<script", "script 不得以元素形态出现")
		assert.NotContains(t, html, "alert(1)", "脚本内容整体丢弃不入出站")
		assert.NotContains(t, html, "onclick", "事件属性不得存活")
		assert.NotContains(t, html, "<!--", "raw HTML omitted 注释被消毒清除")
		assert.Contains(t, html, "bold", "内联标签丢弃但文本保留")
	})

	t.Run("javascript:/data: 链接 scheme 消毒", func(t *testing.T) {
		html, err := RenderRichtext("[点我](javascript:alert(1))\n\n[x](data:text/html;base64,PHNjcmlwdD4=)")
		require.NoError(t, err)
		assert.NotContains(t, html, "javascript:", "危险 scheme 不得存活")
		assert.NotContains(t, html, "data:text/html", "data: HTML 不得存活")
	})

	t.Run("相对路径图片 URL 保留（稳定 URL 硬约束）", func(t *testing.T) {
		html, err := RenderRichtext("![a](/api/v1/files/tickets/a.png)")
		require.NoError(t, err)
		assert.Contains(t, html, `src="/api/v1/files/tickets/a.png"`)
	})

	t.Run("空串渲染为空", func(t *testing.T) {
		html, err := RenderRichtext("")
		require.NoError(t, err)
		assert.Equal(t, "", strings.TrimSpace(html))
	})
}

// 拍板 P4（2026-10-21）：内嵌图仅稳定 URL——外链/协议相对图 src 须被剥除
// （探针实证 bluemonday 把 //host 归入"相对 URL"放行——查看即外联追踪面）；
// 稳定 URL 前缀保留；<a href> 外链保留（点击非自动加载）。
func TestRenderRichtext_ImageSrcWhitelist(t *testing.T) {
	cases := []struct {
		name, src string
		keep      bool
	}{
		{"协议相对外链剥除", "//evil.example.com/track.png", false},
		{"https 外链剥除", "https://evil.example.com/t.png", false},
		{"稳定 URL 保留", "/api/v1/files/tickets/2026/10/a.png", true},
	}
	for _, tc := range cases {
		html, err := RenderRichtext("![" + tc.name + "](" + tc.src + ")")
		require.NoError(t, err, tc.name)
		if tc.keep {
			assert.Contains(t, html, `src="`+tc.src+`"`, tc.name)
		} else {
			assert.NotContains(t, html, tc.src, tc.name+"——src 不得存活")
		}
	}
	// <a href> 外链保留
	html, err := RenderRichtext("[外链](https://example.com/x)")
	require.NoError(t, err)
	assert.Contains(t, html, `href="https://example.com/x"`, "外链 <a> 保留")
}

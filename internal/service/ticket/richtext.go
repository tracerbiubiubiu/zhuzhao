package ticket

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

// richtext 渲染/消毒 SSOT（05 号 §6——B5-4）：存储态 = MD 源文本，出站 HTML 由服务端
// 白名单渲染产出（前端不自行渲染用户输入——防存储型 XSS 的既定口径）。
//
// 选型（2026-10-20 所有者"继续"按建议拍定）：goldmark（CommonMark、Hugo 同款、活跃维护；
// 原本已是本仓间接依赖）+ bluemonday UGC 白名单消毒（成熟、OWASP 推荐）。替代方案
// blackfriday（停止维护）/自写白名单（不值）均否决。
//
// 双保险结构：goldmark 默认（未开 html.WithUnsafe）将 MD 内嵌原始 HTML **整体丢弃**
// （渲染为 `<!-- raw HTML omitted -->` 注释，不参与渲染；实测 2026-10-20）；其产出再过
// bluemonday UGC 白名单二次收口（注释清除 + 链接/图片 scheme 与属性面）。
var mdRenderer = goldmark.New(
	// 05 §6 排版诉求 = 标题/列表/代码块/表格/图片——CommonMark 核心覆盖前四项外，
	// 表格需 GFM Table 扩展（默认不开）
	goldmark.WithExtensions(extension.Table),
)

var mdSanitizer = buildMDSanitizer()

func buildMDSanitizer() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// 内嵌图片 src 为稳定 URL 硬约束（05 §6：/api/v1/files 代理端点——相对路径），
	// 显式放行相对 URL；外链图片 http/https 由 UGC 标准域面覆盖。
	p.AllowRelativeURLs(true)
	p.AllowURLSchemes("http", "https")
	return p
}

// RenderRichtext MD 源文本 → 消毒 HTML（服务端 SSOT 出站）。
func RenderRichtext(src string) (string, error) {
	var buf bytes.Buffer
	if err := mdRenderer.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return mdSanitizer.Sanitize(buf.String()), nil
}

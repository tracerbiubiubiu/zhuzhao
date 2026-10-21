package ticket

import (
	"bytes"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/util"
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
	// 表格需 GFM Table 扩展（默认不开）。
	// 拍板 P4（2026-10-21）：自定义 Image 渲染器覆盖内建——内嵌图仅稳定 URL
	//（/api/v1/files 前缀）产出 <img>，外链/协议相对图（//host）降级为 alt 文本。
	// 探针实证 bluemonday 把 //host 归入"相对 URL"放行、且其属性策略为 OR 合并
	// 无法收紧——在产出点收口是唯一可靠位；bluemonday 仍作第二道（scheme/属性面）。
	goldmark.WithExtensions(extension.Table),
	goldmark.WithRendererOptions(
		renderer.WithNodeRenderers(util.Prioritized(&stableSrcImageRenderer{}, 0)),
	),
)

// ⚠ 保险丝（2026-10-21 所有者核验提醒）：goldmark 渲染器注册=后注册同 Kind 覆盖
// 先注册——stableSrcImageRenderer 是 Image 的全局唯一生效点。后续新增渲染器
//（如 S1 富文本渲染面）须显式链式委托本渲染器（自定义渲染器内调用
// stableSrcImageRenderer.renderImage）而非重新 Register(ast.KindImage,...)，
// 否则外链图白名单静默失效——TestRenderRichtext_ImageSrcWhitelist 为回归闸。

// stableSrcImageRenderer 内嵌图稳定 URL 白名单渲染器（拍板 P4）。
type stableSrcImageRenderer struct{}

func (r *stableSrcImageRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(ast.KindImage, r.renderImage)
}

func (r *stableSrcImageRenderer) renderImage(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
	if !entering {
		return ast.WalkContinue, nil
	}
	n := node.(*ast.Image)
	var alt []byte
	for c := n.FirstChild(); c != nil; c = c.NextSibling() {
		if t, ok := c.(*ast.Text); ok {
			alt = append(alt, t.Segment.Value(source)...)
		}
	}
	if !bytes.HasPrefix(n.Destination, []byte("/api/v1/files/")) {
		// 外链/协议相对图：不产出 <img>（防查看即外联的追踪面），alt 文本降级保语义
		if len(alt) > 0 {
			w.Write(util.EscapeHTML(alt))
		}
		return ast.WalkSkipChildren, nil
	}
	_, _ = w.WriteString(`<img src="`)
	w.Write(util.EscapeHTML(n.Destination))
	_, _ = w.WriteString(`" alt="`)
	w.Write(util.EscapeHTML(alt))
	_, _ = w.WriteString(`">`)
	return ast.WalkSkipChildren, nil
}

var mdSanitizer = buildMDSanitizer()

func buildMDSanitizer() *bluemonday.Policy {
	p := bluemonday.UGCPolicy()
	// 相对 URL 放行为稳定 URL（/api/v1/files）而设；<a href> 外链 http/https
	// 由 UGC 标准域面覆盖（点击行为非自动加载，保留）。
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

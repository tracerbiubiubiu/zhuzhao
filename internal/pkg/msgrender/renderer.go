// Package msgrender 消息模板占位符渲染（S6 B6-3——06 号 A.4）。
//
// 服务端占位符替换（字段值转义）——不用完整模板引擎/text/template，
// 防模板语法执行（用户可控的工单字段值不得被当模板代码）。
// 选型预置 = {{ticket.assignee_name}} 点路径占位符（logic-less 零执行）。
package msgrender

import (
	"fmt"
	"regexp"
	"strings"
)

// 占位符正则：{{ticket.field_key}} 或 {{ticket.field_key.sub}}（点路径）
var placeholderRe = regexp.MustCompile(`\{\{(\w+(?:\.\w+)*)\}\}`)

// Render 渲染模板——将 {{path}} 替换为 vars 中对应值。
// 未命中的占位符保留原样（管理员可见——非静默消除）。
func Render(template string, vars map[string]any) string {
	return placeholderRe.ReplaceAllStringFunc(template, func(m string) string {
		path := strings.Trim(m, "{}")
		val, ok := lookupPath(vars, path)
		if !ok {
			return m // 未命中保留原样（调试可见）
		}
		return fmt.Sprint(val)
	})
}

// lookupPath 点路径取值（如 "ticket.title" → vars["ticket"]["title"]）。
func lookupPath(vars map[string]any, path string) (any, bool) {
	parts := strings.Split(path, ".")
	var cur any = vars
	for _, p := range parts {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		v, exists := m[p]
		if !exists {
			return nil, false
		}
		cur = v
	}
	return cur, true
}

// ExtractPlaceholders 提取模板中的占位符路径集（变量字典校验用）。
func ExtractPlaceholders(template string) []string {
	matches := placeholderRe.FindAllStringSubmatch(template, -1)
	out := make([]string, 0, len(matches))
	for _, m := range matches {
		out = append(out, m[1])
	}
	return out
}

// RenderBoth 渲染主题+正文（便捷函数）。
func RenderBoth(subject, body string, vars map[string]any) (string, string) {
	return Render(subject, vars), Render(body, vars)
}

// EscapeHTML HTML 转义（邮件 HTML 渠道用——用户可控值防注入）。
func EscapeHTML(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return replacer.Replace(s)
}

// RenderEscaped 渠道感知渲染（06 号 A.4 "值转义按渠道进行"）。外审实例批两处修正：
// ① 渠道名对齐真实名空间——原只认 email_html（DDL 默认 channel='uip'，转义对
//
//	真实流量零效果），改显式映射：uip/webhook=纯文本不转义（UIP 契约到手后按
//	实际正文格式重映射此表）；email_html 及未知渠道=HTML 转义（fail-safe）。
//
// ② 值级转义——原对渲染后整串转义（模板静态文本也被转义），改为只转义被替换
//
//	进正文的变量值，模板自身文本原样。
func RenderEscaped(template string, vars map[string]any, channel string) string {
	if !channelEscapes(channel) {
		return Render(template, vars)
	}
	return Render(template, escapeValues(vars))
}

func channelEscapes(channel string) bool {
	switch channel {
	case "uip", "webhook":
		return false
	default:
		return true
	}
}

// escapeValues 深拷贝变量树，字符串叶子做 HTML 转义（值级转义——模板文本不动）。
func escapeValues(vars map[string]any) map[string]any {
	out := make(map[string]any, len(vars))
	for k, v := range vars {
		out[k] = escapeValue(v)
	}
	return out
}

func escapeValue(v any) any {
	switch t := v.(type) {
	case string:
		return EscapeHTML(t)
	case map[string]any:
		return escapeValues(t)
	default:
		return v
	}
}

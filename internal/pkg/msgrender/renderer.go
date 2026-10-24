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

// builtinKeys 内置键集合（06 号 A.4 四象限——这些键必须始终有值，缺失=fail 拒发）。
// 通知链每次组装载荷必含——缺失说明模板或数据有错（非正常场景）。
var builtinKeys = map[string]bool{
	"ticket.title":     true,
	"ticket.type_code": true,
	"ticket.priority":  true,
	"ticket.status":    true,
	"ticket.human_key": true,
	"ticket.link":      true,
}

// RenderResult 渲染结果（四象限三分档——B6-3 增强批）。
type RenderResult struct {
	Text     string   // 渲染后文本
	Err      error    // 内置键缺失→非 nil（拒发信号）
	Warnings []string // 动态键跳过/键在无值 的警告（日志面——不影响发送）
}

// RenderQuadrant 四象限三分档渲染（B6-3 增强批核心件——06 号 A.4）。
//
// 三分档：
//   - 内置键缺失 → Err 非 nil（**fail 拒发**——模板引用了必须有的变量但没有）
//   - 动态键缺失 → 替换为空串 + Warning（**skip+warn**——动态字段可能不在）
//   - 键在但值为 nil → 替换为空串 + Warning（**空串+warned**）
func RenderQuadrant(template string, vars map[string]any) RenderResult {
	var warnings []string
	var renderErr error

	text := placeholderRe.ReplaceAllStringFunc(template, func(m string) string {
		path := strings.Trim(m, "{}")
		val, ok := lookupPath(vars, path)
		if !ok {
			if builtinKeys[path] {
				// 内置键缺失 = fail（拒发——组装一个 error 信号）
				if renderErr == nil {
					renderErr = fmt.Errorf("内置键 %s 缺失——载荷组装错误，拒发", path)
				}
				return m // 保留原样（错误已设置，最终不发送）
			}
			// 动态键缺失 = skip + warn
			warnings = append(warnings, fmt.Sprintf("动态键 %s 未命中——跳过", path))
			return ""
		}
		if val == nil {
			// 键在但无值 = 空串 + warn
			warnings = append(warnings, fmt.Sprintf("键 %s 值为 nil——替换为空串", path))
			return ""
		}
		return fmt.Sprint(val)
	})

	return RenderResult{Text: text, Err: renderErr, Warnings: warnings}
}

// Render 渲染模板——将 {{path}} 替换为 vars 中对应值。
// 未命中的占位符保留原样（管理员可见——非静默消除）。
// 注意：此函数不做四象限分档——旧调用方兼容保留；新调用方应用 RenderQuadrant。
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

// RenderBoth 渲染主题+正文（便捷函数——旧调用方兼容）。
func RenderBoth(subject, body string, vars map[string]any) (string, string) {
	return Render(subject, vars), Render(body, vars)
}

// RenderBothQuadrant 四象限渲染主题+正文（B6-3 增强批——任一 Err 则整体拒发）。
func RenderBothQuadrant(subject, body string, vars map[string]any) (RenderResult, RenderResult) {
	return RenderQuadrant(subject, vars), RenderQuadrant(body, vars)
}

// EscapeHTML HTML 转义（邮件 HTML 渠道用——用户可控值防注入）。
func EscapeHTML(s string) string {
	replacer := strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")
	return replacer.Replace(s)
}

// channelEscapes 渠道→转义族映射（uip/webhook=纯文本不转义；其余=HTML fail-safe）。
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

// RenderQuadrantEscaped 四象限+值级转义（外审 F-1-1 终修：在替换时同步转义值——
// 模板文本原样，仅变量值按渠道转义。消除了"先渲染后全文转义"的粒度问题）。
func RenderQuadrantEscaped(template string, vars map[string]any, channel string) RenderResult {
	if !channelEscapes(channel) {
		return RenderQuadrant(template, vars) // 非转义渠道直接走普通 Quadrant
	}
	// 值级转义：深拷贝变量树，字符串叶子做 HTML 转义，再走 Quadrant
	return RenderQuadrant(template, escapeValues(vars))
}

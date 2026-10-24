// 路由↔menu_apis 对账（BK-22，16 号批次 B）：
// 「有路由无绑定 = 裸奔」（CasbinAuth 面上的路由没有 menu_apis 行——或被通配策略
// 放行人人都可进）；「有绑定无路由 = 死策略」（menu_apis 行指向已不存在的路由，
// AssignMenus 授予它形同虚设）。双向清零 = 权限面与路由面一致。
//
// 执行点 = app.InitializeApp（engine 构建后、启动前）：有缺口即返回错误拒绝启动。
//
// 豁免集（有意不走 CasbinAuth 的路由，新增豁免须在此登记并说明理由）：
//   - 非 /api/v1 路径：探针、/internal/*（AK/SK 验签，非用户面）
//   - 认证公开端点：auth/login、auth/refresh（无角色可调）
//   - 自服务白名单（SelfService 标记）：auth/logout、auth/password/update、
//     user/profile、user/profile/update、user/menus、user/permissions
//   - 委托语义（SelfService，org 管理权限码与组内委托矩阵在 Service 层判定）：
//     orgs/*（orgDelegated 组）
//   - 网关反代前缀（如 /al）：上游路由属跨仓 activelist，zhuzhao 侧不可枚举——
//     该前缀下路由/绑定的对账属 activelist 侧职责（v1 边界，见 16 号批次 B）
package router

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// CatalogGap 单条对账缺口。
type CatalogGap struct {
	Kind   string // "missing_binding"（有路由无绑定=裸奔）| "dead_binding"（有绑定无路由=死策略）
	Method string
	Path   string
}

func (g CatalogGap) String() string {
	return fmt.Sprintf("[%s] %s %s", g.Kind, g.Method, g.Path)
}

// catalogExempt 自服务/公开路径精确豁免集（"METHOD PATH"）。

// CatalogExemptRoutes 豁免登记全集（排序）——导出供守护测试引用
// （SelfServiceContextKey 同款先例；修复验证审计 M-2：豁免路由须伴随
// 运行时放行，防「只登记忘挂标记」的 P0-1 同族复发）。
func CatalogExemptRoutes() []string {
	out := make([]string, 0, len(catalogExempt))
	for k := range catalogExempt {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

var catalogExempt = map[string]bool{
	"GET /api/v1/ticket-types/:code/fields/variables": true, // B6-3 变量字典（模板编辑器读面）
	"GET /api/v1/me/notification-optouts":             true, // B6-3 自服务偏好（登录态）
	"POST /api/v1/me/notification-optouts":            true,

	"POST /api/v1/auth/login":            true, // 认证公开端点
	"POST /api/v1/auth/refresh":          true, // 认证公开端点
	"GET /api/v1/auth/captcha":           true, // P4-7 验证码公开端点（匿名获取——登录前置）
	"GET /api/v1/user/pats":              true, // P4-6 PAT 自服务（SelfService——本人凭据）
	"GET /api/v1/user/dicts/:code/items": true, // P4-3 字典消费面（SelfService——全员业务表单选项）
	// phase5 S5 对象存储自服务面（05 号 A.7——五端点挂 authed 级 SelfService 标记组
	// [router storageSelf]，ticket 态 purpose 的 ticket:update+L2 由 handler 编排层
	// 判定——S5 评审 P0-1 修复 2026-10-20：原挂 biz 无标记致普通角色全 403）
	"POST /api/v1/storage/presign/upload":      true,
	"GET /api/v1/files/*key":                   true, // phase5 S5 文件代理（登录态——A.4 #5；avatar 面可见，ticket 态 handler L2）
	"POST /api/v1/storage/avatar/bind":         true, // B5-2-3 头像回写（SelfService——本人对象）
	"GET /api/v1/tickets/:id/attachments":      true, // B5-3 附件列表（L2 read 面——handler 判定，不可见单 404）
	"POST /api/v1/tickets/attachments/delete":  true, // B5-3 删附件（L2 update 面）
	"POST /api/v1/tickets/attachments/confirm": true,
	"POST /api/v1/tickets/richtext/render":     true, // B5-4 渲染 SSOT（登录态自服务——无票据访问，纯消毒渲染）
	// S10 B10-1 抄送（L2 read/update 面——service 层动态判定，工单操作不走静态权限码）
	"POST /api/v1/tickets/cc":        true,
	"POST /api/v1/tickets/cc/remove": true,
	"GET /api/v1/tickets/:id/cc":     true,
	// S10 B10-2 标签（L2 read/update 面——service 层动态判定；默认共享拍板点 6）
	"POST /api/v1/tickets/tags":        true,
	"POST /api/v1/tickets/tags/remove": true,
	"GET /api/v1/tickets/:id/tags":     true,
	// S10 B10-3 角标聚合（登录态自服务——sections 内不含越权数据）
	"GET /api/v1/badges":                true,
	"POST /api/v1/user/pats":            true,
	"POST /api/v1/user/pats/delete":     true,
	"POST /api/v1/auth/logout":          true, // 自服务（SelfService）
	"POST /api/v1/auth/password/update": true, // 自服务
	"GET /api/v1/user/profile":          true, // 自服务
	"POST /api/v1/user/profile/update":  true, // 自服务
	"GET /api/v1/user/menus":            true, // 自服务
	"GET /api/v1/user/permissions":      true, // 自服务
	"GET /api/v1/user/orgs":             true, // 自服务（P4-W3「我的组织」数据源）
	"POST /api/v1/orgs/delete":          true, // 委托语义（orgDelegated，Service 层判定）
	"POST /api/v1/orgs/members":         true,
	"GET /api/v1/orgs/members/list":     true, // 委托语义（orgDelegated，Service 层 L3 判定——P4-W3 名册）
	"POST /api/v1/orgs/members/role":    true,
	"POST /api/v1/orgs/members/scope":   true,
	"GET /api/v1/orgs/roles/list":       true,
	"POST /api/v1/orgs/roles/bind":      true,
	"POST /api/v1/orgs/roles/delete":    true,
	"POST /api/v1/orgs/owners":          true,
	"POST /api/v1/orgs/members/delete":  true,
}

func exemptPath(method, path string) bool { return catalogExempt[method+" "+path] }

// underGatewayPrefix 路径是否落在任一网关反代前缀下。
func underGatewayPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		if p != "" && strings.HasPrefix(path, p+"/") {
			return true
		}
	}
	return false
}

// boundQuerier menu_apis 读取所需的最小查询接口（*pgxpool.Pool 满足）。
type boundQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// LoadBoundAPIs 载入 menu_apis 绑定集（"METHOD PATH" → true）。
func LoadBoundAPIs(ctx context.Context, q boundQuerier) (map[string]bool, error) {
	rows, err := q.Query(ctx, `SELECT api_method, api_path FROM menu_apis`)
	if err != nil {
		return nil, fmt.Errorf("读取 menu_apis: %w", err)
	}
	defer rows.Close()
	bound := map[string]bool{}
	for rows.Next() {
		var method, path string
		if err := rows.Scan(&method, &path); err != nil {
			return nil, fmt.Errorf("scan menu_apis: %w", err)
		}
		bound[method+" "+path] = true
	}
	return bound, rows.Err()
}

// AuditRouteCatalog 双向对账：engine 已注册路由 × menu_apis 绑定集。
// bound 的 key = "METHOD PATH"（如 "GET /api/v1/users"）。
// gatewayPrefixes：网关反代前缀（该前缀下的路由/绑定互相不可枚举，整体豁免）。
// 返回全部缺口（调用方决定 fail 或报告）。
func AuditRouteCatalog(routes gin.RoutesInfo, bound map[string]bool, gatewayPrefixes []string) []CatalogGap {
	var gaps []CatalogGap

	routeKeys := map[string]bool{}
	allRoutes := map[string]bool{} // 全部已注册路由（含豁免）——死策略判定的比对基准
	for _, rt := range routes {
		allRoutes[rt.Method+" "+rt.Path] = true
		key := rt.Method + " " + rt.Path
		if !strings.HasPrefix(rt.Path, "/api/v1/") {
			continue // 探针/internal 等，非用户面
		}
		if exemptPath(rt.Method, rt.Path) {
			continue
		}
		if underGatewayPrefix(rt.Path, gatewayPrefixes) {
			continue // 跨仓上游，v1 边界
		}
		routeKeys[key] = true
		if !bound[key] {
			gaps = append(gaps, CatalogGap{Kind: "missing_binding", Method: rt.Method, Path: rt.Path})
		}
	}
	for key := range bound {
		method, path, ok := splitBoundKey(key)
		if !ok {
			continue
		}
		if !strings.HasPrefix(path, "/api/v1/") || underGatewayPrefix(path, gatewayPrefixes) {
			continue
		}
		// 死策略 = 绑定指向的路由完全不存在（豁免路由真实存在，其绑定不算死）
		if !allRoutes[key] {
			gaps = append(gaps, CatalogGap{Kind: "dead_binding", Method: method, Path: path})
		}
	}
	return gaps
}

func splitBoundKey(key string) (method, path string, ok bool) {
	i := strings.IndexByte(key, ' ')
	if i <= 0 || i == len(key)-1 {
		return "", "", false
	}
	return key[:i], key[i+1:], true
}

// FormatGaps 缺口清单（启动错误信息用）。
func FormatGaps(gaps []CatalogGap) string {
	var b strings.Builder
	for _, g := range gaps {
		b.WriteString("\n  " + g.String())
	}
	return b.String()
}

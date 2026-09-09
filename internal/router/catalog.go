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
var catalogExempt = map[string]bool{
	"POST /api/v1/auth/login":           true, // 认证公开端点
	"POST /api/v1/auth/refresh":         true, // 认证公开端点
	"POST /api/v1/auth/logout":          true, // 自服务（SelfService）
	"POST /api/v1/auth/password/update": true, // 自服务
	"GET /api/v1/user/profile":          true, // 自服务
	"POST /api/v1/user/profile/update":  true, // 自服务
	"GET /api/v1/user/menus":            true, // 自服务
	"GET /api/v1/user/permissions":      true, // 自服务
	"POST /api/v1/orgs/delete":          true, // 委托语义（orgDelegated，Service 层判定）
	"POST /api/v1/orgs/members":         true,
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

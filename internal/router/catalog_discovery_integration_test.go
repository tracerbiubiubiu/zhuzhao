//go:build integration

// BK-22 发现跑：真实路由（router.New 全量注册）× 真实 menu_apis（testutil 迁移链）。
// 缺口非零 = 本测试失败——修 seed（死策略）或补路由绑定（裸奔）或登记豁免，
// 修完 app 层 fail-fast（InitializeApp）才会放行启动。
package router

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCatalogDiscovery_RealRoutesRealMenuAPIs(t *testing.T) {
	pool := testPool // 生命周期归 TestMain（main_integration_test.go）

	engine := New(Deps{DBPool: pool}) // handler/中间件依赖可 nil——注册期不解引用

	bound, err := LoadBoundAPIs(context.Background(), pool)
	require.NoError(t, err)

	gaps := AuditRouteCatalog(engine.Routes(), bound, []string{"/al"}) // /al = 网关反代前缀（v1 跨仓豁免）
	for _, g := range gaps {
		t.Logf("BK-22 GAP %v", g)
	}
	// 测试库刻意排除 000002 纯种子（user/role/menu/org 绑定行缺失属预期口径），
	// 故此处只 guard 死策略（含入清单 migrations 的绑定行必须指向真实路由）；
	// missing_binding（裸奔）由 wire 层启动 fail-fast 在全量种子库上执行。
	var dead []CatalogGap
	for _, g := range gaps {
		if g.Kind == "dead_binding" {
			dead = append(dead, g)
		}
	}
	require.Empty(t, dead, "存在死策略（menu_apis 指向不存在路由）——修 seed 或删行（清单见 GAP 日志）")
}

// B 案守卫（E2E 补齐批四程）：页面/目录行（menu_type≠3）不得承载非 GET 绑定——写端点一律挂按钮行，
// 例外须带设计出处登记 pageWriteAllow（当前唯一：ticket_list transition，000068）。
// 根因史：000070/000073 把分派规则三写挂页面行→viewer 预设即可建/停/删规则（000075 修）；
// 000034/000059 字典×6/通知×8 同病（000077 修）——约定靠人记不住，改为启动 fail-fast + 本测试双守卫。
func TestCatalogPageRows_NoWriteBindings(t *testing.T) {
	viol, err := AuditPageWriteBindings(context.Background(), testPool)
	require.NoError(t, err)
	require.Empty(t, viol, "B 案违例：页面/目录行承载写端点——迁按钮行（000075/000077 同款）或带设计出处登记 pageWriteAllow：%s",
		FormatPageWriteBindings(viol))
}

//go:build integration

// BK-22 发现跑：真实路由（router.New 全量注册）× 真实 menu_apis（testutil 迁移链）。
// 缺口非零 = 本测试失败——修 seed（死策略）或补路由绑定（裸奔）或登记豁免，
// 修完 app 层 fail-fast（InitializeApp）才会放行启动。
package router

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

func TestCatalogDiscovery_RealRoutesRealMenuAPIs(t *testing.T) {
	pool, cleanup, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	t.Cleanup(cleanup)

	engine := New(Deps{DBPool: pool}) // handler/中间件依赖可 nil——注册期不解引用

	bound, err := LoadBoundAPIs(context.Background(), pool)
	require.NoError(t, err)

	gaps := AuditRouteCatalog(engine.Routes(), bound, []string{"/al"}) // /al = 网关反代前缀（v1 跨仓豁免）
	for _, g := range gaps {
		t.Logf("BK-22 GAP %v", g)
	}
	require.Empty(t, gaps, "路由↔menu_apis 存在漂移——修 seed 或补豁免（清单见上方 GAP 日志）")
}

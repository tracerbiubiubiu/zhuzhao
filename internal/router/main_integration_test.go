//go:build integration

package router

import (
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

// testPool 全包共享 PG（testutil 迁移链）——两道 catalog 守卫共用。
// 此前包内仅一条集成测试、由其自持 t.Cleanup；加第二条后首条的 Cleanup 会先关掉共享池，
// 后者查询即报 closed pool——改为 TestMain 持有生命周期（handler/repository/service 同款）。
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup, err := testutil.SetupPostgresShared()
	if err != nil {
		panic(err)
	}
	testPool = pool
	code := m.Run()
	cleanup()
	os.Exit(code)
}

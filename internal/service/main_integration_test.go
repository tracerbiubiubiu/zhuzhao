//go:build integration

package service_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

var testPool *pgxpool.Pool

// uniqueSuffix 测试数据唯一后缀（测试隔离债治理，2026-09-01）：
// 完整 UnixNano，无周期回绕。禁止再引入 %1e9 等截断形式——截断值的周期性
// 回绕是同模式隐患（ticket 包的 'MS' 截断即该模式的 23505 事故实例）。
func uniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

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

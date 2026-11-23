//go:build integration

package risk_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

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

// uniqueSuffix 测试数据唯一后缀（全量 UnixNano，无截断回绕——仓内隔离债纪律）。
func uniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

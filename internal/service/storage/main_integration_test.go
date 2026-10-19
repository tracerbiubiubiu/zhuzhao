//go:build integration

package storage_test

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

var testPool *pgxpool.Pool

// uniqueSuffix 测试数据唯一后缀（测试隔离债治理惯例：完整 UnixNano，禁截断形式）。
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

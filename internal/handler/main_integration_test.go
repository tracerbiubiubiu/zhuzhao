//go:build integration

package handler_test

import (
	"os"
	"testing"

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

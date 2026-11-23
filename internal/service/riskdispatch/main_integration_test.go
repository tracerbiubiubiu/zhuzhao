//go:build integration

package riskdispatch

import (
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

var dispatchTestDB *pgxpool.Pool

func TestMain(m *testing.M) {
	db, cleanup, err := testutil.SetupPostgresShared()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	dispatchTestDB = db
	code := m.Run()
	cleanup()
	os.Exit(code)
}

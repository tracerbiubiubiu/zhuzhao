//go:build integration

package testutil

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

var (
	sharedOnce sync.Once
	sharedPool *pgxpool.Pool
	sharedTerm func()
	sharedErr  error
)

// SetupPostgresShared 全包共享一个 PG 容器（TestMain 用）。
func SetupPostgresShared() (*pgxpool.Pool, func(), error) {
	sharedOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()

		container, err := postgres.Run(ctx,
			"postgres:15-alpine",
			postgres.WithDatabase("zhuzhao_test"),
			postgres.WithUsername("zhuzhao"),
			postgres.WithPassword("zhuzhao_test"),
			testcontainers.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(3*time.Minute),
			),
		)
		if err != nil {
			sharedErr = err
			return
		}

		connStr, err := container.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			sharedErr = err
			return
		}

		pool, err := pgxpool.New(ctx, connStr)
		if err != nil {
			sharedErr = err
			return
		}
		if err := runInitMigration(ctx, pool); err != nil {
			sharedErr = err
			pool.Close()
			return
		}

		sharedPool = pool
		sharedTerm = func() {
			pool.Close()
			_ = container.Terminate(context.Background())
		}
	})
	return sharedPool, sharedTerm, sharedErr
}

// SetupPostgres 单测独立容器（慢，优先用 TestMain + SetupPostgresShared）。
func SetupPostgres(t testingTB) *pgxpool.Pool {
	t.Helper()
	pool, cleanup, err := SetupPostgresShared()
	if err != nil {
		t.Fatalf("setup postgres: %v", err)
	}
	t.Cleanup(cleanup)
	return pool
}

type testingTB interface {
	Helper()
	Fatalf(string, ...any)
	Cleanup(func())
}

func runInitMigration(ctx context.Context, pool *pgxpool.Pool) error {
	path := initMigrationPath()
	sql, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(sql))
	return err
}

func initMigrationPath() string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "migrations/000001_init.up.sql"
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations", "000001_init.up.sql")
}

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
		// 000001 建表 + 000006 唯一索引软删过滤（F-6），
		// 保证测试 schema 与生产迁移后的语义一致（软删工号/域账号可复用）
		for _, name := range []string{"000001_init.up.sql", "000006_partial_unique_indexes.up.sql"} {
			if err := runMigration(ctx, pool, name); err != nil {
				sharedErr = err
				pool.Close()
				return
			}
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

func runMigration(ctx context.Context, pool *pgxpool.Pool, name string) error {
	sql, err := os.ReadFile(migrationPath(name))
	if err != nil {
		return err
	}
	_, err = pool.Exec(ctx, string(sql))
	return err
}

func migrationPath(name string) string {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		return "migrations/" + name
	}
	return filepath.Join(filepath.Dir(file), "..", "..", "migrations", name)
}

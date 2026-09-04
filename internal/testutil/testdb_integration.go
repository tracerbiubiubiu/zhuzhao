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
		// 迁移列表与生产保持同步（D2-31：原仅 000001/000006/000008——
		// casbin_rule 列名漂移 p_type vs 生产 ptype，2a 引入真实 adapter
		// 集成测试时会爆雷）。排除 000002/000007（种子/存量数据修复，
		// 测试自建数据，避免幻影行）；后续新增迁移须同步此列表
		//（phase2/00-implementation-plan §5.3 检查单第 5 条）
		for _, name := range []string{
			"000001_init.up.sql",
			"000003_casbin_column.up.sql",
			"000004_casbin_column_ptype.up.sql",
			"000005_casbin_nullable.up.sql",
			"000006_partial_unique_indexes.up.sql",
			"000008_user_orgs_single_primary.up.sql",
			"000009_phase1_hardening.up.sql",
			// 2a Step 2：工单表（D1）+ 工单菜单种子（D2，已并入 000010_ticket）
			"000010_ticket.up.sql",
			// 2b-core Step 4：organizations.ticket_visibility（09 §5.2.1）
			"000011_ticket_visibility.up.sql",
			// 2b-org Step 5：来源列 + user_orgs.ticket_scope/expires_at + org_roles + roles.parent_id
			"000012_org_enhance.up.sql",
			// 2c Step 8：organizations.owner_user_ids + user_orgs.org_member_role
			"000013_org_delegation.up.sql",
			// 2c 收口：ticket_events 去 CASCADE（HC2 审计完整性）
			"000014_audit_events_nocascade.up.sql",
			// 2a Step 3：工单模板 + 工单关联（2a 前移）
			"000015_ticket_templates.up.sql",
			"000016_ticket_relations.up.sql",
			// IW1/BK-13：project_isolated CHECK 放开（09 §5.2.1 强隔离开关）
			"000017_ticket_visibility_check.up.sql",
			// IW3/BK-18：类型管理闭环（validate_regex 列 + 管理页菜单/menu_apis）
			"000018_ticket_type_admin.up.sql",
			// org_type 四值枚举收敛为 is_virtual 布尔（000019，层级由 path/nlevel 表达）
			"000019_org_is_virtual.up.sql",
			// B11① 判定日志表 + audit_logs/ticket_events 补 request_id（000020）
			"000020_policy_eval_request_id.up.sql",
		} {
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

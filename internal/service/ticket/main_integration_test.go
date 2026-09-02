//go:build integration

package ticket

import (
	"context"
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

	// 2a 集成测试前置：补充最小 ticket_types 种子行（incident/request）
	// 注：000010_ticket_menu.up.sql 的 role_menus 通配 INSERT 跳过 000002 种子（§5.3）
	// 所以这里还需手动确保 roles.id、menus.id 存在：
	// roles 与 menus 表在 000001_init.up.sql + 000010_ticket_menu.up.sql 已建表。
	// role_menus 种子（000010_ticket_menu.up.sql）依赖 roles.code，
	// 但 000002_seed.up.sql 被 SetupPostgresShared 排除了——测试自行 INSERT role 行。
	ctx := context.Background()
	seedMinimal(ctx, pool)

	code := m.Run()
	cleanup()
	os.Exit(code)
}

// uniqueSuffix 测试数据唯一后缀（测试隔离债治理，2026-09-01）：
// 完整 UnixNano，无周期回绕。禁止再引入 %1e9 / to_char(...,'MS') 等截断形式——
// 截断值的周期性回绕是 idx_org_code 23505 flaky 的根因（见 00 检查单随手项「测试隔离债」）。
func uniqueSuffix() string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

// softDeleteOrg 测试结束软删 setup 建的 org：idx_org_code 是 WHERE deleted_at IS NULL
// 的部分唯一索引，软删即释放 code；FK 不看 deleted_at，无需先清理引用表。
func softDeleteOrg(t *testing.T, orgID int64) {
	t.Helper()
	t.Cleanup(func() {
		if _, err := testPool.Exec(context.Background(), `UPDATE organizations SET deleted_at = NOW() WHERE id = $1`, orgID); err != nil {
			t.Logf("cleanup: soft-delete org %d: %v", orgID, err)
		}
	})
}

// seedMinimal 补最小集合（与 000002_seed 同语义但精简，以保证测试稳定、无幻影行）
func seedMinimal(ctx context.Context, pool *pgxpool.Pool) {
	// 1. roles 基础行（先于 menus/role_menus 因为 000010_ticket_menu.up.sql 结尾 FK 绑定 role_menus）
	_, err := pool.Exec(ctx, `INSERT INTO roles (code, name, description, priority, status, sort_order, is_system) VALUES
	('superadmin', '超级管理员', '', 1, 1, 1, true),
	('admin',      '系统管理员', '', 2, 1, 2, true),
	('operator',   '普通操作员', '', 3, 1, 3, true),
	('viewer',     '只读查看者', '', 4, 1, 4, true)
	ON CONFLICT DO NOTHING`)
	if err != nil {
		panic(err)
	}

	// 2. organizations 根行（code 为部分唯一索引 WHERE deleted_at IS NULL，不指定列让 PG 自动推断）
	var rootID int64
	err = pool.QueryRow(ctx, `INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ('root', '根组织', NULL, 'root'::ltree, 1, 1, 1, true)
		ON CONFLICT (code) WHERE deleted_at IS NULL DO UPDATE SET code=EXCLUDED.code
		RETURNING id`).Scan(&rootID)
	if err != nil {
		panic(err)
	}
	_ = rootID

	// 3. ticket_types 基础行（Create() 依赖 type_code 存在，GetTicketType NOT NULL 检查）
	// incident 用 DDL DEFAULT transitions（含 rejected→open，无 reassigned）
	// request 显式写不同 transitions（无 rejected，有 reassigned→assigned）——体现流程差异化
	// 两个类型的 transitions 不同是 TestTypeDiversity_DBTransitionsDiffer 的前提
	_, err = pool.Exec(ctx, `INSERT INTO ticket_types (code, name, description, transitions) VALUES
		('incident', '事件工单', '故障/事件处理', DEFAULT),
		('request',  '请求工单', '服务请求/咨询',
		 '{"open":["assigned","closed"],"assigned":["in_progress","open","reassigned"],"in_progress":["pending_verify","closed"],"pending_verify":["closed","reassigned"],"closed":["open"],"reassigned":["assigned"]}'::jsonb)
		ON CONFLICT (code) DO UPDATE SET transitions = EXCLUDED.transitions`)
	if err != nil {
		panic(err)
	}
}

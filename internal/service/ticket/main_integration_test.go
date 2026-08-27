//go:build integration

package ticket

import (
	"context"
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

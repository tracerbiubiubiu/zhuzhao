//go:build integration

package risk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

// 合成夹具（详设 §13/§14 命名）：G=91001 虚拟风险组（M 管理职责）、D=91002 受影响部门
// （V 副锚成员）、U=92002 责任人；本批全部为合成数据，不指向任何真实组织/人员。
const (
	orgGID        = int64(91001) // 风险管理组织（虚拟风险组）
	orgDID        = int64(91002) // 受影响部门
	sourceRef     = "mock-vulnerability"
	sourceMapping = "mapping:v1"
)

// 合成主体用变量（测试需取地址构造可空指针字段）。
var (
	userM int64 = 92001 // 主锚管理者（org G admin）
	userU int64 = 92002 // 资产责任人（owner）
	userV int64 = 92004 // 副锚只读成员（org D member）
	userN int64 = 92006 // 有 L1 但范围为空
)

// seedRiskFixture 幂等落合成组织/人员/成员资格与合成来源登记。
func seedRiskFixture(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `
INSERT INTO organizations(id, code, name, path, status, is_system, is_virtual)
VALUES ($1,'risk-g','合成风险组G','org91001',1,false,true),
       ($2,'risk-d','合成受影响部门D','org91002',1,false,false)
ON CONFLICT DO NOTHING`, orgGID, orgDID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `
INSERT INTO users(id, username, password, real_name, status)
VALUES ($1,'risk_m','fixture','合成管理者M',1),
       ($2,'risk_u','fixture','合成责任人U',1),
       ($3,'risk_v','fixture','合成副锚V',1),
       ($4,'risk_n','fixture','合成无范围N',1)
ON CONFLICT DO NOTHING`, userM, userU, userV, userN)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `
INSERT INTO user_orgs(user_id, org_id, org_member_role, ticket_scope, is_primary)
VALUES ($1,$2,'admin','group',true),
       ($3,$4,'member','group',true),
       ($5,$2,'member','assigned',false),
       ($1,$4,'admin','group',false)
ON CONFLICT (user_id, org_id) DO NOTHING`, userM, orgGID, userV, orgDID, userU)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `
INSERT INTO risk_sources(source_ref, display_name, fingerprint_scope, ordered, mapping_ref, allowed_org_ids, default_org_id, enabled, note)
VALUES ($1,'合成漏洞来源','mock-vulnerability',true,$2,ARRAY[$3::bigint],$3,true,'本轮合成来源夹具')
ON CONFLICT (source_ref) DO NOTHING`, sourceRef, sourceMapping, orgGID)
	require.NoError(t, err)
	// 显式 ID 落库后推进序列，避免同包后续隐式插入撞主键。
	_, err = testPool.Exec(ctx, `SELECT setval('organizations_id_seq', GREATEST((SELECT MAX(id) FROM organizations),1));
SELECT setval('users_id_seq', GREATEST((SELECT MAX(id) FROM users),1))`)
	require.NoError(t, err)
}

// seedCasbinRole 建合成角色/操作者并授指定 POST 路由（供 C04 HTTP 守卫测试）。
// 中间件内存策略与 DB p 行同时落数据——撤权测试删 DB 行后内存策略仍在，
// 以此证明服务端在写事务内对当前授权的重验。
func seedCasbinRole(t *testing.T, paths ...string) (roleCode string, actorID int64) {
	t.Helper()
	ctx := context.Background()
	roleCode = "risk_op_" + uniqueSuffix()
	var roleID int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO roles(code,name,priority)VALUES($1,$1,50)RETURNING id`, roleCode).Scan(&roleID))
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO users(username,password,real_name,status)VALUES($1,'fixture','合成操作者',1)RETURNING id`,
		"op_"+roleCode).Scan(&actorID))
	_, err := testPool.Exec(ctx,
		`INSERT INTO user_roles(user_id,role_id)VALUES($1,$2)`, actorID, roleID)
	require.NoError(t, err)
	// 操作者需对合成风险组 G 具主锚管理资格（L3 管理范围），
	// 使 happy 路径越过 L3 后由守卫内的凭据/策略重验给出判别。
	_, err = testPool.Exec(ctx, `
INSERT INTO user_orgs(user_id, org_id, org_member_role, ticket_scope)
VALUES ($1,$2,'admin','group')
ON CONFLICT (user_id, org_id) DO NOTHING`, actorID, orgGID)
	require.NoError(t, err)
	for _, p := range paths {
		_, err = testPool.Exec(ctx,
			`INSERT INTO casbin_rule(ptype,v0,v1,v2)VALUES('p',$1,$2,'POST')`, "role::"+roleCode, p)
		require.NoError(t, err)
	}
	return roleCode, actorID
}

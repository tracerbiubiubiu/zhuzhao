//go:build integration

package ticket

// 2c Step 9（M2c-2）集成测试：工单 Authorize 升级 D7–D9 + 委托不越容器边界。
// SSOT: docs/phase2/04-org-delegation.md §4.2/§7。

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// setupD9 环境：P(实体, owner=pOwner) > vg(虚拟组, owner/admin/member) [+ 可选 vg2 兄弟]
type d9Env struct {
	svc    *Service
	pID    int64
	vgID   int64
	owner  int64 // vg owner（org_member_role=owner）
	admin  int64 // vg admin
	member int64 // vg member（工单创建人）
	pOwner int64 // P 的实体 owner（owner_user_ids，ancestor owner）
	roles  stubRoleFetcher
}

func setupD9(t *testing.T) *d9Env {
	t.Helper()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1e9)
	svc, _, _, _, roles := setupTicket2a(t)
	ctx := context.Background()

	// 组织树 root > P(实体) > vg(虚拟组)
	var pID, vgID int64
	pCode := "p2c9p_" + suffix
	vgCode := "vg_2c9_" + suffix
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ($1, 'P', 1, $2::ltree, 3, 1, 75, false) RETURNING id`,
		pCode, "root."+pCode).Scan(&pID))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ($1, 'VG', $2, $3::ltree, 4, 1, 75, false) RETURNING id`,
		vgCode, pID, "root."+pCode+"."+vgCode).Scan(&vgID))

	mkuser := func(name string) int64 {
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
			INSERT INTO users (username, password, employee_no, status) VALUES ('%s', 'hash', 'E%s', 1)
			RETURNING id`, name, name)).Scan(&id))
		roles[id] = []string{"operator"}
		return id
	}
	mkvgMember := func(name, role string) int64 {
		id := mkuser(name)
		_, err := testPool.Exec(ctx, `
			INSERT INTO user_orgs (user_id, org_id, is_primary, org_member_role) VALUES ($1, $2, false, $3)`,
			id, vgID, role)
		require.NoError(t, err)
		return id
	}

	// P 实体 owner（ancestor owner）。双轨对齐：owner 经成员行挂 P（生产 SetOwners
	// 行为）——成员行同时提供 L2 锚点（P 实体透明读 → 可见子树 vg 工单）
	pOwner := mkuser("p2c9pown_" + suffix)
	_, err := testPool.Exec(ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary, org_member_role) VALUES ($1, $2, false, 'owner')`,
		pOwner, pID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids = ARRAY[$1]::bigint[] WHERE id = $2`, pOwner, pID)
	require.NoError(t, err)

	return &d9Env{
		svc: svc, pID: pID, vgID: vgID,
		owner:  mkvgMember("p2c9own_"+suffix, "owner"),
		admin:  mkvgMember("p2c9adm_"+suffix, "admin"),
		member: mkvgMember("p2c9mem_"+suffix, "member"),
		pOwner: pOwner, roles: roles,
	}
}

// D7/D8：vg admin/owner（非创建人）凭组内委托可 update/close/delete 本组工单；
// 非创建人 member 不可（委托不扩 member）
func TestD9_D7D8_OrgAdminOperatesTicket(t *testing.T) {
	env := setupD9(t)
	ctx := context.Background()

	tk := newTicketHelper(t, env.svc, env.member, env.vgID, "D7 委托工单")

	// D7：vg admin update（RK-11 的「仅创建人」对委托者豁免——04 §4.2 update 行）
	updated, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("admin 委托改名")}, env.admin)
	require.NoError(t, err, "vg admin 凭组内委托应可 update 本组工单")
	assert.Equal(t, "admin 委托改名", updated.Title)

	// D7 变体：vg owner 亦可
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("owner 委托改名")}, env.owner)
	require.NoError(t, err)

	// admin close（open→closed 合法转换）+ delete（owner 档）
	require.NoError(t, env.svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, env.admin))
	tk2 := newTicketHelper(t, env.svc, env.member, env.vgID, "D7 delete 工单")
	require.NoError(t, env.svc.Delete(ctx, tk2.ID, env.owner))

	// D8：非创建人 member → 403（委托不扩 member）
	tk3 := newTicketHelper(t, env.svc, env.owner, env.vgID, "D8 工单")
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk3.ID, Title: strPtr("越权")}, env.member)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)
}

// D9：实体部门 owner（P，非 vg 成员）凭 ancestor owner 管子树 vg 工单（update/delete）
func TestD9_D9_AncestorOwnerSubtree(t *testing.T) {
	env := setupD9(t)
	ctx := context.Background()

	tk := newTicketHelper(t, env.svc, env.member, env.vgID, "D9 子树工单")

	updated, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("实体 owner 委托改名")}, env.pOwner)
	require.NoError(t, err, "实体 owner 凭 ancestor owner 应可管子树工单（D9）")
	assert.Equal(t, "实体 owner 委托改名", updated.Title)

	require.NoError(t, env.svc.Delete(ctx, tk.ID, env.pOwner))
}

// 边界（04 §4.2 注/D11）：vg admin 不能改兄弟虚拟组工单——委托以 org 精确匹配为界
func TestD9_VgAdminCannotCrossVg(t *testing.T) {
	env := setupD9(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1e9)
	ctx := context.Background()

	vg2Code := "vg_2c9b_" + suffix
	var vg2ID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ($1, 'VG2', $2, (SELECT path::text || '.' || $3 FROM organizations WHERE id = $2)::ltree, 4, 1, 75, false)
		RETURNING id`, vg2Code, env.pID, vg2Code).Scan(&vg2ID))

	// member（vg1）在 vg2 建单?不行——成员关系在 vg1。直接用 vg2 成员建单：
	var vg2Member int64
	require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO users (username, password, employee_no, status) VALUES ('p2c9m2_%s', 'hash', 'Ep2c9m2%s', 1)
		RETURNING id`, suffix, suffix)).Scan(&vg2Member))
	_, err := testPool.Exec(ctx, `
		INSERT INTO user_orgs (user_id, org_id, is_primary, org_member_role) VALUES ($1, $2, false, 'member')`,
		vg2Member, vg2ID)
	require.NoError(t, err)
	env.roles[vg2Member] = []string{"operator"}

	tk := newTicketHelper(t, env.svc, vg2Member, vg2ID, "vg2 工单")

	// vg1 admin 对 vg2 工单：透明读可见（D11 L2），但委托不匹配 → 403
	_, err = env.svc.Get(ctx, tk.ID, env.admin)
	require.NoError(t, err, "L2 透明读不受委托边界影响")
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("跨组越权")}, env.admin)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)
}

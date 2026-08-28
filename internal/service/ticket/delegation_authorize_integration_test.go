//go:build integration

package ticket

// 2c Step 9（M2c-2）集成测试：工单 Authorize 升级 D7–D9 + 委托不越容器边界。
// SSOT: docs/phase2/04-org-delegation.md §4.2/§7。

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
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

	// HC2：deleted 事件随库存活（FK ON DELETE SET NULL → ticket_id 悬空）
	require.NoError(t, env.svc.Delete(ctx, tk.ID, env.pOwner))
	var delEvents int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ticket_events WHERE action='deleted' AND user_id=$1 AND ticket_id IS NULL`,
		env.pOwner).Scan(&delEvents))
	assert.GreaterOrEqual(t, delEvents, 1, "删除应写 deleted 事件（SET NULL 后随库存活）")
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

// Assign closed 回归：closed 工单不可分派（90004，与 Update 对齐）
func TestD9_AssignClosedTicket90004(t *testing.T) {
	env := setupD9(t)
	ctx := context.Background()
	tk := newTicketHelper(t, env.svc, env.member, env.vgID, "closed assign 工单")
	require.NoError(t, env.svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, env.member))

	// vg owner 有 assign 委托权（IsOrgAdminOrOwner）→ 命中 closed 守卫 → 90004
	err := env.svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &env.owner}, env.owner)
	requireErrCode(t, err, errcode.ErrTicketAlreadyClosed.Code)
}

// D9 move 回归：实体 owner 所在部门被 move 后，子树委托随之迁移（新 path 下仍有效）
func TestD9_AncestorOwnerAfterMove(t *testing.T) {
	env := setupD9(t)
	ctx := context.Background()
	tk := newTicketHelper(t, env.svc, env.member, env.vgID, "move 后委托工单")

	// move P 到 root 直下（path 级联，owner_user_ids 随 org 行保留）
	orgRepo := repository.NewOrgRepo(testPool)
	rootID := int64(1)
	require.NoError(t, orgRepo.Move(ctx, env.pID, &rootID))

	// ancestor owner（P.owner）对新 path 下子树工单仍可 update（D9 语义经 move 存续）
	_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("move 后 owner 改名")}, env.pOwner)
	require.NoError(t, err, "P2-D1 级联后 ancestor owner 委托应随新 path 继续")

	// vg admin 对本组工单有委托权（D7），不受 move 影响仍可改
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: strPtr("vg admin 改名")}, env.admin)
	require.NoError(t, err, "vg admin 本组委托不受 move 影响")
}

// CC1/CC2 回归：并发 Close+Assign —— 两者竞争同一 open 工单，
// 最终态必须 ∈ {closed, assigned} 且禁止「closed 复活为 assigned」；
// 双并发 Close 只允许一次成功（无 closed→closed 脏事件）
func TestD9_ConcurrentCloseAssignRace(t *testing.T) {
	env := setupD9(t)
	ctx := context.Background()

	for iter := 0; iter < 10; iter++ {
		tk := newTicketHelper(t, env.svc, env.member, env.vgID, fmt.Sprintf("竞态-%d", iter))

		var closeErr, assignErr error
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			closeErr = env.svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, env.member)
		}()
		go func() {
			defer wg.Done()
			assignErr = env.svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &env.owner}, env.member)
		}()
		wg.Wait()

		got, err := env.svc.Get(ctx, tk.ID, env.member)
		require.NoError(t, err)
		switch got.Status {
		case "closed":
			// Close 赢：Assign 必须失败（90004）且不产生 assigned_to 变更
			assert.Error(t, assignErr, "close 赢时 assign 必须被拒")
			assert.Nil(t, got.AssignedTo, "closed 态不得残留新处理人")
			assert.Nil(t, closeErr)
		case "assigned":
			// Assign 赢：Close 必须失败（SM 断言基于旧状态 → 90002/90004/10006 之一均可，关键是不落 closed）
			assert.Error(t, closeErr, "assign 赢时 close 必须被拒")
		default:
			t.Fatalf("竞态后出现非法终态 %q（iter=%d）", got.Status, iter)
		}
	}
}

// CC1 回归：双并发 Close —— 恰一次成功，另一次 90004/409，且 status_changed 事件仅一条
func TestD9_ConcurrentDoubleClose(t *testing.T) {
	env := setupD9(t)
	ctx := context.Background()

	for iter := 0; iter < 10; iter++ {
		tk := newTicketHelper(t, env.svc, env.member, env.vgID, fmt.Sprintf("双关-%d", iter))

		var errs [2]error
		var wg sync.WaitGroup
		wg.Add(2)
		for i := range errs {
			go func(i int) {
				defer wg.Done()
				errs[i] = env.svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, env.member)
			}(i)
		}
		wg.Wait()

		var okCnt int
		for _, e := range errs {
			if e == nil {
				okCnt++
			}
		}
		assert.Equal(t, 1, okCnt, "双并发 Close 恰一次成功（iter=%d）", iter)

		var evCount int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM ticket_events WHERE ticket_id=$1 AND action='status_changed' AND to_value='closed'`,
			tk.ID).Scan(&evCount))
		assert.Equal(t, 1, evCount, "closed 事件不得重复写入（iter=%d）", iter)
	}
}

// TC2/MC1 回归：工单关联 Go 层用例——正向建联、同向唯一（409）、
// 目标被物理删除后建联 → 400（23503 映射，非 500）
func TestD9_CreateRelation(t *testing.T) {
	env := setupD9(t)
	ctx := context.Background()

	tkA := newTicketHelper(t, env.svc, env.member, env.vgID, "关联源工单")
	tkB := newTicketHelper(t, env.svc, env.member, env.vgID, "关联目标工单")

	// 正向：双端同属创建人 → 200
	rel, err := env.svc.CreateRelation(ctx, &model.CreateRelationRequest{
		SourceTicketID: tkA.ID, TargetTicketID: tkB.ID, RelationType: "related",
	}, env.member)
	require.NoError(t, err)
	assert.Equal(t, tkA.ID, rel.SourceTicketID)

	// 同向重复 → 409（部分唯一索引）
	_, err = env.svc.CreateRelation(ctx, &model.CreateRelationRequest{
		SourceTicketID: tkA.ID, TargetTicketID: tkB.ID, RelationType: "related",
	}, env.member)
	requireErrCode(t, err, errcode.ErrConflict.Code)

	// MC1：目标被物理删除后建联 → 统一 404+90001（不可见语义，防枚举；
	// 23503 → MapForeignKeyViolation 为纵深防御，正常流被鉴权先行拦截）
	tkG := newTicketHelper(t, env.svc, env.member, env.vgID, "将被删除的工单")
	require.NoError(t, env.svc.Delete(ctx, tkG.ID, env.admin)) // vg admin 有 delete 权（04 §4.2）
	_, err = env.svc.CreateRelation(ctx, &model.CreateRelationRequest{
		SourceTicketID: tkA.ID, TargetTicketID: tkG.ID,
	}, env.member)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
}

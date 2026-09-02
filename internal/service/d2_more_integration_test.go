//go:build integration

package service_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// d2TestMenu 建一个 type=2 页面菜单（带 component/icon）
func d2TestMenu(t *testing.T, code, component, icon string) *model.Menu {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
		INSERT INTO menus (code, name, menu_type, path, component, icon, sort_order, visible, is_system)
		VALUES ($1, $1, 2, $2, $3, $4, 1, true, false) RETURNING id`,
		code, "/d2/"+strings.ToLower(code), component, icon).Scan(&id))
	return &model.Menu{ID: id, Code: code, MenuType: 2}
}

// D2-17 守护：菜单 Update patch 语义——未传 component/icon/path 不得清空
// （原全量覆盖零值穿透；B2-3 只改了用户模块）
func TestMenuService_UpdatePatchSemantics(t *testing.T) {
	resetOrgTables(t)
	ctx := context.Background()
	svc := service.NewMenuService(repository.NewMenuRepo(testPool), repository.NewUserRepo(testPool), repository.NewRoleRepo(testPool))

	menu := d2TestMenu(t, "d2_page", "system/d2/index", "star")

	// ① 只改名（未传 path/component/icon/permission/sort_order）→ 均保持现值
	updated, err := svc.Update(ctx, &model.UpdateMenuRequest{
		ID: menu.ID, Version: 1, Name: "改名后",
	})
	require.NoError(t, err)
	assert.Equal(t, "system/d2/index", updated.Component, "未传 component 不得清空")
	assert.Equal(t, "star", updated.Icon, "未传 icon 不得清空")
	assert.Equal(t, "/d2/d2_page", updated.Path, "未传 path 不得清空")
	assert.Equal(t, 1, updated.SortOrder, "未传 sort_order 不得归零")

	// ② 显式传空串 → 才清空（patch 语义不阻碍显式操作）
	empty := ""
	updated, err = svc.Update(ctx, &model.UpdateMenuRequest{
		ID: menu.ID, Version: 2, Name: "改名后", Component: &empty, Icon: &empty,
	})
	require.NoError(t, err)
	assert.Empty(t, updated.Component)
	assert.Empty(t, updated.Icon)
}

// D2-14 守护：AssignMenus 入参重复 menu_id → 去重后成功
// （原 ListByIDs 去重行数 ≠ len 误报 ErrMenuNotFound；B3-4 只给 SetUserOrgsTx 做了同型）
// 角色用系统 admin（superadmin actor 直通目标校验；admin 角色跳过 Casbin 写入，
// 规避测试 enforcer 无 adapter——同 TestRBACService_AssignMenus_SystemRoleRequiresSuperadmin 基建约定）
func TestRBACService_AssignMenusDuplicateIDs(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc := newRBACServiceForTest(t)

	adminRole := rbacTestRole(t, "admin", 10, true)
	saRole := rbacTestRole(t, "superadmin", 1, true)
	actorID := rbacTestUser(t, "d2_sa", "E632001", []int64{saRole.ID})
	m1 := d2TestMenu(t, "d2_dup_m1", "", "")
	m2 := d2TestMenu(t, "d2_dup_m2", "", "")

	// 重复传 m1 两次——修复前：len(active)=2 != 3 → 404
	err := svc.AssignMenus(ctx, &model.AssignMenusRequest{
		RoleID: adminRole.ID, MenuIDs: []int64{m1.ID, m1.ID, m2.ID},
	}, actorID)
	require.NoError(t, err, "重复 menu_id 应去重成功而非误报 404")

	var bound int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM role_menus WHERE role_id = $1`, adminRole.ID).Scan(&bound))
	assert.Equal(t, 2, bound, "重复 ID 只落一次绑定")
}

// D2-16 守护：SetUserOrgsTx 命中软删组织 → 404 + 事务回滚（不留部分绑定）
// （原裸 INSERT 触发 FK 23503 → 500；B4-3 只给 SetRolesTx 做了同型防御）
func TestOrgRepo_SetUserOrgsTxSoftDeletedOrg(t *testing.T) {
	resetOrgTables(t)
	ctx := context.Background()
	orgRepo := repository.NewOrgRepo(testPool)
	userRepo := repository.NewUserRepo(testPool)

	var aliveID, deadID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path, is_virtual, status, is_system)
		VALUES ('d2_alive', '存活', 'd2_alive', false, 1, false) RETURNING id`).Scan(&aliveID))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path, is_virtual, status, is_system, deleted_at)
		VALUES ('d2_dead', '已软删', 'd2_dead', false, 1, false, NOW()) RETURNING id`).Scan(&deadID))

	user := &model.User{Username: "d2_suo", EmployeeNo: "E632002", Password: "$2a$12$dummydummydummydummydummydummydummydummydummydu", Status: 1}
	require.NoError(t, userRepo.Create(ctx, user))

	err := orgRepo.SetUserOrgs(ctx, user.ID, []int64{aliveID, deadID}, nil)
	requireErrCode(t, err, errcode.ErrOrgNotFound)

	// 事务回滚：存活组织也不得落库（不留部分写）
	var n int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE user_id = $1`, user.ID).Scan(&n))
	assert.Zero(t, n, "失败路径必须整体回滚，不得留下部分绑定")
}

// D2-26 守护：SetRolesTx 去重 + TOCTOU 窗口内软删角色 → 404 回滚
// （service 预检 FindByID 先挡一层；此处直调 repo 验证行数核对的纵深防御）
func TestUserRepo_SetRolesTxDedupAndSoftDeleted(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(testPool)

	role := rbacTestRole(t, "d2_sr", 20, false)
	softDeleted := rbacTestRole(t, "d2_sr_gone", 20, false)
	_, err := testPool.Exec(ctx, `UPDATE roles SET deleted_at = NOW() WHERE id = $1`, softDeleted.ID)
	require.NoError(t, err)

	actor := rbacTestUser(t, "d2_sr_actor", "E632003", nil)

	// ① 重复 roleID → 去重成功
	require.NoError(t, userRepo.SetRoles(ctx, actor, []int64{role.ID, role.ID}))
	var n int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_roles WHERE user_id = $1`, actor).Scan(&n))
	assert.Equal(t, 1, n, "重复 roleID 只落一次绑定")

	// ② 软删角色 → 404 + 不产生幽灵绑定
	err = userRepo.SetRoles(ctx, actor, []int64{role.ID, softDeleted.ID})
	requireErrCode(t, err, errcode.ErrRoleNotFound)
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_roles WHERE user_id = $1 AND role_id = $2`, actor, softDeleted.ID).Scan(&n))
	assert.Zero(t, n, "软删角色不得写入绑定（原静默跳过留半套）")
}

// D2-15 守护：并发双 AddMember(is_primary=true) 不同组织的**不变量**——
// 无论调度如何交错：双方均不得 500，终态恰好一个 primary（000008 部分唯一索引兜底）。
// 真 23505→50011 映射路径与 SetUserOrgsTx 共用 mapUniqueViolation（B3-3 已覆盖）；
// 黑盒下 INSERT 重叠窗口（两 UPDATE 均先于两 INSERT）不稳定，不在此断言特定错误码。
func TestOrgRepo_ConcurrentDualPrimary(t *testing.T) {
	resetOrgTables(t)
	ctx := context.Background()
	orgRepo := repository.NewOrgRepo(testPool)
	userRepo := repository.NewUserRepo(testPool)

	var orgA, orgB int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path, is_virtual, status, is_system)
		VALUES ('d2_ca', '甲', 'd2_ca', false, 1, false) RETURNING id`).Scan(&orgA))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path, is_virtual, status, is_system)
		VALUES ('d2_cb', '乙', 'd2_cb', false, 1, false) RETURNING id`).Scan(&orgB))

	// 无任何绑定的用户：两事务并发置 primary
	user := &model.User{Username: "d2_race", EmployeeNo: "E632004", Password: "$2a$12$dummydummydummydummydummydummydummydummydummydu", Status: 1}
	require.NoError(t, userRepo.Create(ctx, user))

	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, orgID := range []int64{orgA, orgB} {
		wg.Add(1)
		go func(id int64) {
			defer wg.Done()
			errs <- orgRepo.AddMember(ctx, id, user.ID, true)
		}(orgID)
	}
	wg.Wait()
	close(errs)

	for err := range errs {
		if err == nil {
			continue
		}
		// 失败方只允许是业务错误（50011），绝不允许裸 500（D2-15 修复目标）
		var biz *errcode.Error
		require.ErrorAs(t, err, &biz, "并发冲突必须是业务错误而非 500：%v", err)
		assert.Equal(t, errcode.ErrDuplicatePrimaryOrg.Code, biz.Code)
	}

	// 终态不变量：恰好一个 primary（索引兜底，无论谁赢）
	var primaries int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE user_id = $1 AND is_primary`, user.ID).Scan(&primaries))
	assert.Equal(t, 1, primaries)
}

// D2-44④ 守护：组织层级超过 20 → 400（原 ltree 拼超深 path 报 500）
func TestOrgService_CreateDepthLimit(t *testing.T) {
	resetOrgTables(t)
	ctx := context.Background()
	svc := service.NewOrgService(repository.NewOrgRepo(testPool), repository.NewUserRepo(testPool), service.NewOrgDelegationService(testPool), newRBACServiceForTest(t))

	// 直插 20 段 path 的父组织（绕过 service 限制构造深层基线）
	deepPath := strings.Repeat("l.", 19) + "l20" // 20 段 label
	var parentID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path, is_virtual, status, is_system)
		VALUES ('d2_deep', '深层', $1, false, 1, false) RETURNING id`, deepPath).Scan(&parentID))

	_, err := svc.Create(ctx, &model.CreateOrgRequest{
		Code: "d2_too_deep", Name: "超限", ParentID: &parentID, IsVirtual: false,
	}, 1)
	requireErrCode(t, err, errcode.ErrInvalidParams)
}

// D2-18 守护：用户列表分页回显与实查 clamp 一致（请求 200 实查 100 回显 100）
func TestUserService_ListPageSizeEchoClamp(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	svc, _, _ := newUserServiceForD2(t)

	saRole := rbacTestRole(t, "superadmin", 1, true)
	actor := rbacTestUser(t, "d2_echo", "E632005", []int64{saRole.ID})

	resp, err := svc.List(ctx, repository.UserListQuery{Page: 1, PageSize: 200}, actor)
	require.NoError(t, err)
	assert.Equal(t, 100, resp.PageSize, "回显必须与实查 clamp 后一致（原回显 200）")
	assert.Equal(t, 1, resp.Page)
}

// D2-21 守护：用户名搜索 ILIKE 通配符转义——'a%b' 只精确匹配 a%b，不把 axb 卷入
func TestUserRepo_ListILIKEEscape(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	userRepo := repository.NewUserRepo(testPool)

	dummy := "$2a$12$dummydummydummydummydummydummydummydummydummydu"
	for _, name := range []string{"a%b", "axb", "a_b", "ayb"} {
		require.NoError(t, userRepo.Create(ctx, &model.User{
			Username: name, EmployeeNo: "E63201" + name, Password: dummy, Status: 1,
		}))
	}

	users, _, err := userRepo.List(ctx, repository.UserListQuery{Username: "a%b"})
	require.NoError(t, err)
	assert.Len(t, users, 1, "%% 通配符须按字面匹配（修复前 a%%b 同时命中 axb/ayb）")
	assert.Equal(t, "a%b", users[0].Username)

	users, _, err = userRepo.List(ctx, repository.UserListQuery{Username: "a_b"})
	require.NoError(t, err)
	assert.Len(t, users, 1, "_ 单字符通配符须按字面匹配")
	assert.Equal(t, "a_b", users[0].Username)
}

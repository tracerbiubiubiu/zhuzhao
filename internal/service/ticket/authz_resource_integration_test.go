//go:build integration

package ticket

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// stubRoleFetcher 直接按 userID→[]string 返回角色码（摆脱 Casbin/RBAC 全链路）。
type stubRoleFetcher map[int64][]string

func (s stubRoleFetcher) GetRoleCodesByUserID(_ context.Context, userID int64) ([]string, error) {
	if v, ok := s[userID]; ok {
		return v, nil
	}
	return []string{}, nil
}

// stubRoleFetcher 在编译期校验满足 middleware.RoleFetcher 接口
var _ middleware.RoleFetcher = stubRoleFetcher{}

// setupTicket2a 准备 R3-R8 真表环境：
//
//	返回 svc、辅助 ID、role fetcher 指针（可在单测内改映射模拟 superadmin）。
func setupTicket2a(t *testing.T) (*Service, int64, int64, int64, stubRoleFetcher) {
	t.Helper()
	ctx := context.Background()

	ticketRepo := repository.NewTicketRepo(testPool)
	orgRepo := repository.NewOrgRepo(testPool)
	reg := resource.NewRegistry()
	reg.Register(NewResource(ticketRepo, NewStubScopeResolver()))

	roles := stubRoleFetcher{}
	svc := NewTicketService(testPool, ticketRepo, orgRepo, reg, roles)

	// 预取 root org id（seedMinimal 已建）
	var rootID int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id FROM organizations WHERE code='root' LIMIT 1`).Scan(&rootID))

	// 建子 org "p2a_it"
	var orgID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ('p2a_it_'+to_char(clock_timestamp(),'MS'), '2a Integration', $1,
		        (SELECT path::text||'.p2a_it'::text FROM organizations WHERE id=$1)::ltree,
		        3, 1, 90, false)
		RETURNING id`).Scan(&orgID))
	// 若 ltree 绑定失败（字符串拼接），退化：直接取 tech 最后 id
	if orgID == 0 {
		require.NoError(t, testPool.QueryRow(ctx, `
			INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
			VALUES ('p2a_it_sub','2a IT Sub',$1,'root.p2a_it_sub'::ltree,3,1,90,false)
			ON CONFLICT DO NOTHING RETURNING id`).Scan(&orgID))
	}

	// 建用户：A(operator 创建者) / B(operator 处理人) / V(viewer)
	var aid, bid, vid int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO users (username,password,employee_no,status) VALUES ('2a_it_a','hash','E2A001',1) RETURNING id`).Scan(&aid))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO users (username,password,employee_no,status) VALUES ('2a_it_b','hash','E2A002',1) RETURNING id`).Scan(&bid))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO users (username,password,employee_no,status) VALUES ('2a_it_v','hash','E2A003',1) RETURNING id`).Scan(&vid))

	// 绑定角色（由 stubRoleFetcher 控制，无需 user_roles 行；R3-R8 不依赖 DB 角色关系，由 stub 注入）
	roles[aid] = []string{"operator"}
	roles[bid] = []string{"operator"}
	roles[vid] = []string{"viewer"}

	return svc, aid, bid, vid, roles
}

// newTicketHelper 以 actorUserID 创建一张 incident/open 工单，返回其 id。
func newTicketHelper(t *testing.T, svc *Service, actorID, orgID int64, title string) *model.Ticket {
	t.Helper()
	tk, err := svc.Create(context.Background(), &model.CreateTicketRequest{
		TypeCode: "incident", Title: title, Description: "x", Priority: 3, OrgID: orgID,
	}, actorID)
	require.NoError(t, err, "创建工单失败：title=%s actor=%d", title, actorID)
	return tk
}

// requireErrCode 要求 err 含 errcode.Error 且 code 等于 expect
func requireErrCode(t *testing.T, err error, expect int) {
	t.Helper()
	require.Error(t, err)
	var ee *errcode.Error
	require.True(t, errors.As(err, &ee), "err 应为 errcode.Error：%v (%T)", err, err)
	assert.Equal(t, expect, ee.Code, "错误码不匹配：%v", ee)
}

// rootOrgID 读 root org id
func rootOrgID(t *testing.T) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT id FROM organizations WHERE code='root'`).Scan(&id))
	return id
}

// childOrgID 建 root 下一个子 org 并返回 id
func childOrgID(t *testing.T, suffix string) int64 {
	t.Helper()
	rid := rootOrgID(t)
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
		INSERT INTO organizations (code,name,parent_id,path,org_type,status,sort_order,is_system)
		VALUES ($1,$2,$3,'root.'||$1::text,3,1,50,false)
		ON CONFLICT DO NOTHING RETURNING id`, "p2a_"+suffix, "2a Child "+suffix, rid).Scan(&id))
	return id
}

// ---------- R3：列表 assigned 过滤 ----------
func TestTicket_R3_AssignedScopeList(t *testing.T) {
	svc, aid, bid, _, _ := setupTicket2a(t)
	ctx := context.Background()
	oid := childOrgID(t, "r3")

	// A 建 1 张，B 建 2 张，均不指派
	a1 := newTicketHelper(t, svc, aid, oid, "A-R3-a")
	_ = newTicketHelper(t, svc, bid, oid, "B-R3-b1")
	_ = newTicketHelper(t, svc, bid, oid, "B-R3-b2")

	// A 查询：仅看到自己创建的 1 张
	aList, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50}, aid)
	require.NoError(t, err)
	assert.EqualValues(t, 1, aList.Total, "A 仅应看到自身创建的工单（assigned scope 2a 口径）")
	if aList.Total > 0 {
		assert.Equal(t, a1.ID, aList.List[0].ID)
	}

	// B 查询：看到自身创建的 2 张
	bList, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50}, bid)
	require.NoError(t, err)
	assert.EqualValues(t, 2, bList.Total)
}

// ---------- R4：A 读 B 的工单（不可见）→ 404（非 403）----------
func TestTicket_R4_InvisibleReturns404(t *testing.T) {
	svc, aid, bid, _, _ := setupTicket2a(t)
	oid := childOrgID(t, "r4")
	b1 := newTicketHelper(t, svc, bid, oid, "B-R4-b1")

	// A 尝试 GET → 期望 ErrTicketNotFound (90001)
	_, err := svc.Get(context.Background(), b1.ID, aid)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
}

// ---------- R5：A 更新自己的工单 → 200（成功）----------
func TestTicket_R5_UpdateOwn(t *testing.T) {
	svc, aid, _, _, _ := setupTicket2a(t)
	oid := childOrgID(t, "r5")
	a1 := newTicketHelper(t, svc, aid, oid, "A-R5-a")

	newTitle := "A-R5-updated"
	up, err := svc.Update(context.Background(), &model.UpdateTicketRequest{
		ID:    a1.ID,
		Title: &newTitle,
	}, aid)
	require.NoError(t, err)
	assert.Equal(t, newTitle, up.Title)
	// 再查一次库确认已落盘
	got, err := svc.Get(context.Background(), a1.ID, aid)
	require.NoError(t, err)
	assert.Equal(t, newTitle, got.Title)
}

// ---------- R6：A 更新/分派/删除 可见但非属主的工单 → 403 + 70001 ----------
// 构造「可见但非属主」：由 superadmin 把 B 的工单分派给 A（assigned scope 命中，A 能读），
// 但 canOperate(update/assign/delete) 在 2a 对非属主分派用户 或 非 admin 一律 → 403。
func TestTicket_R6_VisibleNotOwner_403(t *testing.T) {
	svc, aid, bid, vid, roles := setupTicket2a(t)
	oid := childOrgID(t, "r6")
	ctx := context.Background()

	b1 := newTicketHelper(t, svc, bid, oid, "B-R6-b1")
	// 用 aid 的 roles 临时加 superadmin 一次完成分派（再还原）
	oldRoles := roles[aid]
	// 直接用 bid as superadmin 更干净——把 bid 升级为 admin
	roles[bid] = append(roles[bid], "admin")
	err := svc.Assign(ctx, &model.AssignTicketRequest{ID: b1.ID, AssignedTo: &aid}, bid)
	require.NoError(t, err, "admin 分派应该成功")
	roles[bid] = oldRoles // 还原（如果 admin 是追加的旧值其实已经是 ["operator"]，这里把 bid 回退成 operator）
	// 直接通过 roles[bid] = oldRoles 但 oldRoles 是 ["operator"], 而上面 append 改了 roles[bid]
	// 所以还原为 operator-only：
	roles[bid] = []string{"operator"}

	// -- A 分派（自己非 admin，不能分派，哪怕工单可见）--
	err = svc.Assign(ctx, &model.AssignTicketRequest{ID: b1.ID, AssignedTo: &bid}, aid)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	// -- A 删除（canOperate(delete) → 非属主/处理人？不，canOperate(delete) 一律 false） --
	err = svc.Delete(ctx, b1.ID, aid)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	_ = vid // 预留未来扩展 viewer 分派（也应 403，但 R6 已覆盖 assign/delete）
}

// ---------- R7：admin 读任意工单（bypass L2 assigned 过滤）----------
func TestTicket_R7_AdminBypass(t *testing.T) {
	svc, aid, bid, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "r7")

	b1 := newTicketHelper(t, svc, bid, oid, "B-R7-b1")

	// B 创建的工单：普通操作员 aid 默认无法 GET
	_, err := svc.Get(context.Background(), b1.ID, aid)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)

	// 把 aid 升级为 admin → GET 成功
	roles[aid] = []string{"admin"}
	got, err := svc.Get(context.Background(), b1.ID, aid)
	require.NoError(t, err)
	assert.Equal(t, b1.ID, got.ID)

	// 列表：admin 应含 B 创建的工单（assigned scope 对 admin 不过滤）
	list, err := svc.List(context.Background(), model.TicketListQuery{Page: 1, PageSize: 50}, aid)
	require.NoError(t, err)
	found := false
	for _, x := range list.List {
		if x.ID == b1.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "admin 列表应包含他人创建的工单（R7 bypass）")
}

// ---------- R8：viewer 无路由权限 → service 层表现（注意 service 层无法测 L1 Casbin，
// 需要在 HTTP 层测——此处模拟：当角色被从「无 ticket 菜单」切到「有资源级」但保持 stub，
// Service.Authorize/Authorize 对 viewer + 无 ticket:list 菜单的拒绝行为。
// 在 service 层 2a 对应表现：viewer 同样走 assigned scope（A/VID 的 assigned scope 仍正常），
// 但菜单级拒绝在 L1 Casbin 层（HTTP 中间件），不在 service 层——R8 真正断言应走 HTTP acceptance 脚本。
//
// 为与 B4 落点约定保持一致，Service 集成层仅验证：当 A（viewer）被分派他人工单时，
// L2 assigned scope 能正确命中（后续由 L1 Casbin 在中间件层拒掉，实现分离式断言）。
func TestTicket_R8_ViewerServiceLayerAssigned(t *testing.T) {
	svc, aid, bid, vid, roles := setupTicket2a(t)
	oid := childOrgID(t, "r8")
	ctx := context.Background()

	a1 := newTicketHelper(t, svc, aid, oid, "A-R8-a1")
	// 由 bid 作为 admin 把 A 的工单指派给 V（viewer）
	roles[bid] = []string{"admin"}
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: a1.ID, AssignedTo: &vid}, bid))
	roles[bid] = []string{"operator"}

	// V（viewer）查询列表：能看到该工单（通过 assigned）—这是 service 层口径，
	// 真实 R8=403 在路由中间件 Casbin 层：acceptance-phase2a.sh T7 段断言。
	vList, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 50}, vid)
	require.NoError(t, err)
	assert.EqualValues(t, 1, vList.Total)
	_, err = svc.Get(ctx, a1.ID, vid)
	require.NoError(t, err, "viewer 资源级 assigned 命中时 service 允许 GET（L1 在 HTTP 层拦截）")
}

// ---------- 延伸 T6：非法 transition assigned→closed → 90002 ----------
func TestTicket_T6_InvalidTransitionReturns90002(t *testing.T) {
	svc, aid, bid, _, roles := setupTicket2a(t)
	oid := childOrgID(t, "t6")
	ctx := context.Background()

	a1 := newTicketHelper(t, svc, aid, oid, "A-T6-a1")
	// admin 指派给 B → status 变 assigned
	roles[bid] = []string{"admin"}
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: a1.ID, AssignedTo: &bid}, bid))
	roles[bid] = []string{"operator"}

	// B 以普通 operator 非 admin 调用 close：canOperate(close) 对 处理人 (assigned_to=B) 允许（2a canOperate close=creator or assignee），
	// 但状态机 assigned→closed 不在允许列表 → 90002
	err := svc.Close(ctx, &model.CloseTicketRequest{ID: a1.ID, Comment: "直接关"}, bid)
	requireErrCode(t, err, errcode.ErrTicketInvalidTransition.Code)

	// 对比：合法路径 —— 先回 open（取消指派），再 close（合法 open→closed）
	roles[bid] = []string{"admin"}
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: a1.ID, AssignedTo: nil}, bid))
	roles[bid] = []string{"operator"}
	err = svc.Close(ctx, &model.CloseTicketRequest{ID: a1.ID, Comment: "合法关闭"}, aid)
	require.NoError(t, err, "open→closed 种子 transitions 允许")
}

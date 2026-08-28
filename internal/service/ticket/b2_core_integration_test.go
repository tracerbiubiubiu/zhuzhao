//go:build integration

package ticket

// 2b-core（Step 4，M2b-core）集成测试：策略 B 实体透明读机制。
// SSOT: docs/phase2/09-ticket.md §5.2 / §5.2.1。
// 覆盖：同部门同事透明读（D11 机制）/ 跨子树隔离 / RK-11 update 收窄 /
// BK-1 内部备注读过滤 / ticket_visibility 列生效 / 锚点精确性（父组织工单不可见）。
// R9/R10（vg_a/vg_b 虚拟组兄弟）需 Step 5 虚拟组（000012）后在 M2b 验收补齐。

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// b2Env 2b-core 测试环境：P 下两个兄弟实体部门 D1/D2，U1∈D1、U2∈D2、U3∈D1（同事）
type b2Env struct {
	svc       *Service
	pID       int64 // 父部门
	d1, d2    int64 // 兄弟实体部门
	u1, u2    int64 // 部门成员
	colleague int64 // D1 的另一成员（透明读旁观者）
	roles     stubRoleFetcher
	// 2b-org 扩展（setupB2Org）：P 下虚拟组兄弟对及其成员
	vgA, vgB int64
	uVa, uVb int64
}

func setupB2(t *testing.T) *b2Env {
	t.Helper()
	svc, _, _, _, roles := setupTicket2a(t)
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1e9)

	// 父部门 P（root 下）+ 兄弟实体部门 D1/D2（P 下）
	pID := createB2Org(t, rootOrgID(t), "p2bp_"+suffix, "2b 父部门")
	d1 := createB2Org(t, pID, "p2bd1_"+suffix, "2b 部门一")
	d2 := createB2Org(t, pID, "p2bd2_"+suffix, "2b 部门二")

	// 部门成员：U1/U3 ∈ D1，U2 ∈ D2
	u1 := createB2User(t, "p2bu1_"+suffix, "B2U1"+suffix)
	u2 := createB2User(t, "p2bu2_"+suffix, "B2U2"+suffix)
	colleague := createB2User(t, "p2bu3_"+suffix, "B2U3"+suffix)
	bindOrgMember(t, u1, d1)
	bindOrgMember(t, u2, d2)
	bindOrgMember(t, colleague, d1)
	roles[u1] = []string{"operator"}
	roles[u2] = []string{"operator"}
	roles[colleague] = []string{"operator"}

	return &b2Env{svc: svc, pID: pID, d1: d1, d2: d2, u1: u1, u2: u2, colleague: colleague, roles: roles}
}

// T-2b-1（D11 机制）：同部门同事透明可读；跨子树部门不可见；List 行级过滤同步生效
func TestB2_ColleagueTransparentRead(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()

	// U1 在 D1 建单；U2 在 D2 建单
	tkD1 := newTicketHelper(t, env.svc, env.u1, env.d1, "D1 工单")
	tkD2 := newTicketHelper(t, env.svc, env.u2, env.d2, "D2 工单")

	// 同部门同事（U3，透明读，非属主）可读 U1 的 D1 工单
	got, err := env.svc.Get(ctx, tkD1.ID, env.colleague)
	require.NoError(t, err, "同实体部门同事应透明可读（策略 B）")
	assert.Equal(t, tkD1.ID, got.ID)

	// 跨子树（D2 成员读 D1 工单）不可见 → 404 + 90001
	_, err = env.svc.Get(ctx, tkD1.ID, env.u2)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)

	// 父组织 P 本身的工单对 D1 成员不可见——锚点为 D1（最近实体祖先），不含父级
	tkParent := newTicketHelper(t, env.svc, env.u1, env.pID, "P 工单")
	_, err = env.svc.Get(ctx, tkParent.ID, env.colleague)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)

	// List 行级过滤：U3 列表含 D1 工单、不含 D2 工单
	list, err := env.svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 100}, env.colleague)
	require.NoError(t, err)
	ids := map[int64]bool{}
	for _, tk := range list.List {
		ids[tk.ID] = true
	}
	assert.True(t, ids[tkD1.ID], "List 应含同部门透明读工单")
	assert.False(t, ids[tkD2.ID], "List 不应含跨子树工单")
}

// T-2b-2（R12 + D11 不可改半边）：创建人可改自己的（R12）；
// 同部门透明读旁观者可见但不可改（403）；处理人 update 收窄为 403（RK-11）
func TestB2_WriteSeparation(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()

	tk := newTicketHelper(t, env.svc, env.u1, env.d1, "读写分离工单")

	// R12：创建人改自己的 → 200
	updated, err := env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: tk.ID, Title: strPtr("R12 创建人改名"),
	}, env.u1)
	require.NoError(t, err)
	assert.Equal(t, "R12 创建人改名", updated.Title)

	// D11 不可改：透明读旁观者（同事）update → 403（可见但无权限）
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: tk.ID, Title: strPtr("旁观者越权改名"),
	}, env.colleague)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	// RK-11 回归：处理人（非创建人）update → 403（2a 曾放行，2b 收窄）
	env.roles[env.u1] = []string{"admin"} // admin 分派
	require.NoError(t, env.svc.Assign(ctx, &model.AssignTicketRequest{
		ID: tk.ID, AssignedTo: &env.colleague,
	}, env.u1))
	env.roles[env.u1] = []string{"operator"}
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: tk.ID, Title: strPtr("处理人越权改名"),
	}, env.colleague)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	// 处理人 close 仍允许（2b 语义不变）：Assign 流程下 assignee 必伴随 assigned 状态
	//（assigned→closed 非法），可达路径是「创建即分派」（status=open + assignee）——
	// 以该形态验证 isAssignee 的 close 权（open→closed 合法转换）
	assigned := int64(env.colleague)
	tk2, err := env.svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: "incident", Title: "处理人关闭正例", OrgID: env.d1, AssignedTo: &assigned,
	}, env.u1)
	require.NoError(t, err)
	require.NoError(t, env.svc.Close(ctx, &model.CloseTicketRequest{ID: tk2.ID}, env.colleague))
}

// T-2b-3（BK-1 门禁）：内部备注仅 创建人/处理人/admin 可见可写；
// 透明读旁观者仅见公开回复、不可写备注
func TestB2_InternalNoteVisibility(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()

	tk := newTicketHelper(t, env.svc, env.u1, env.d1, "备注可见性工单")

	// 同事（透明读）写公开回复：canRead 放行
	_, err := env.svc.CreateComment(ctx, &model.CreateCommentRequest{
		TicketID: tk.ID, Content: "同事公开回复",
	}, env.colleague)
	require.NoError(t, err)

	// 创建人写内部备注
	_, err = env.svc.CreateNote(ctx, &model.CreateNoteRequest{
		TicketID: tk.ID, Content: "内部：涉密信息",
	}, env.u1)
	require.NoError(t, err)

	// 透明读旁观者 ListComments → 仅公开回复（内部备注被过滤）
	cList, err := env.svc.ListComments(ctx, tk.ID, env.colleague)
	require.NoError(t, err)
	assert.Len(t, cList, 1, "旁观者应只见公开回复")
	for _, c := range cList {
		assert.False(t, c.IsInternal, "旁观者不应见到内部备注")
	}

	// 旁观者写内部备注 → 403（可见但无权限，errDenied → ErrNoPermission）
	_, err = env.svc.CreateNote(ctx, &model.CreateNoteRequest{
		TicketID: tk.ID, Content: "旁观者备注",
	}, env.colleague)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)

	// 创建人可见全部（1 公开 + 1 内部）
	own, err := env.svc.ListComments(ctx, tk.ID, env.u1)
	require.NoError(t, err)
	assert.Len(t, own, 2)
}

// T-2b-4：ticket_visibility 列生效——实体设非透明值后锚点收窄（列无 CHECK，模拟 future 隔离值）
func TestB2_TicketVisibilityColumn(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()

	tk := newTicketHelper(t, env.svc, env.u1, env.d1, "可见性开关工单")

	// 默认透明：同事可读
	_, err := env.svc.Get(ctx, tk.ID, env.colleague)
	require.NoError(t, err)

	// 模拟 future 隔离值（2b-core 无 CHECK，值可写入；GetFilter 无分支，
	// 锚点查询按 ticket_visibility 过滤后自然收窄）
	_, err = testPool.Exec(ctx,
		`UPDATE organizations SET ticket_visibility = 'project_isolated' WHERE id = $1`, env.d1)
	require.NoError(t, err)

	// 同事失去锚点 → 404；创建人（属主）不受影响
	_, err = env.svc.Get(ctx, tk.ID, env.colleague)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	_, err = env.svc.Get(ctx, tk.ID, env.u1)
	require.NoError(t, err, "属主可见性不受 ticket_visibility 影响")
}

// T-2b-5（BK-6 / P2-D1 回归）：组织 move 后工单 org_path 级联重映射，
// 且透明读过滤在**新路径**下仍正确——覆盖级联后代分支（subpath ELSE 分支，
// 此前全仓零覆盖：脚本层仅测叶子节点 move 的 THEN 分支）。
func TestB2_MoveCascadeRemapsDescendantTicketPath(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()

	// D1 下建子部门 D1C，U1 在 D1C 建单（org_path 含三代：P.D1.D1C）
	d1c := createB2Org(t, env.d1, "p2bd1c_"+fmt.Sprintf("%d", time.Now().UnixNano()%1e9), "2b 部门一子级")
	tk := newTicketHelper(t, env.svc, env.u1, d1c, "级联重映射工单")

	oldTicketPath := tk.OrgPath
	// 同事（D1 成员）透明可读（旧路径）
	_, err := env.svc.Get(ctx, tk.ID, env.colleague)
	require.NoError(t, err)

	// move：把 D1 从 P 下挪到 root 直下（后代 D1C 与工单随之重映射）
	orgRepo := repository.NewOrgRepo(testPool)
	rootID := rootOrgID(t)
	require.NoError(t, orgRepo.Move(ctx, env.d1, &rootID))

	// 断言 ELSE 分支：工单 org_path = newRoot(D1 新路径) || subpath(旧路径去掉旧 D1 前缀)
	var d1NewPath, ticketPath string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT path::text FROM organizations WHERE id = $1`, env.d1).Scan(&d1NewPath))
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT org_path::text FROM tickets WHERE id = $1`, tk.ID).Scan(&ticketPath))
	oldD1Path := oldTicketPath[:len(oldTicketPath)-len("."+lastLabel(oldTicketPath))]
	assert.NotEqual(t, oldD1Path, d1NewPath, "D1 应已换位")
	want := d1NewPath + oldTicketPath[len(oldD1Path):]
	assert.Equal(t, want, ticketPath, "后代工单 org_path 应按 newRoot||subpath 重映射")

	// move 后 scope 过滤仍正确：同事经新锚点仍透明可读；跨子树 U2 仍不可见
	_, err = env.svc.Get(ctx, tk.ID, env.colleague)
	require.NoError(t, err, "move 后透明读应基于重映射后的路径继续生效")
	_, err = env.svc.Get(ctx, tk.ID, env.u2)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
}

// lastLabel 取 ltree 路径最后一段标签
func lastLabel(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '.' {
			return path[i+1:]
		}
	}
	return path
}

// T-2b-6（BK-2/BK-3 回归）：Assign 走状态机 + Update 条件更新/事件留痕
func TestB2_AssignStateMachineAndUpdateGuard(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano()%1e9)

	// BK-2：自定义类型 transitions 无 open→assigned → Assign 应 90002（而非静默成功）
	smType := "p2bsm_" + suffix
	_, err := testPool.Exec(ctx, `
		INSERT INTO ticket_types (code, name, transitions) VALUES ($1, '状态机测试', '{"open":["closed"]}'::jsonb)
		ON CONFLICT (code) DO NOTHING`, smType)
	require.NoError(t, err)
	tk := newTicketWithType(t, env.svc, env.u1, env.d1, smType, "状态机工单")

	env.roles[env.u1] = []string{"admin"} // assign 动作仅 admin bypass
	err = env.svc.Assign(ctx, &model.AssignTicketRequest{
		ID: tk.ID, AssignedTo: &env.colleague,
	}, env.u1)
	requireErrCode(t, err, errcode.ErrTicketInvalidTransition.Code)

	// 放开 open→assigned 后同请求成功（证明拦截来自状态机而非其它路径）
	_, err = testPool.Exec(ctx,
		`UPDATE ticket_types SET transitions = '{"open":["assigned"]}'::jsonb WHERE code = $1`, smType)
	require.NoError(t, err)
	require.NoError(t, env.svc.Assign(ctx, &model.AssignTicketRequest{
		ID: tk.ID, AssignedTo: &env.colleague,
	}, env.u1))
	got, err := env.svc.Get(ctx, tk.ID, env.u1)
	require.NoError(t, err)
	assert.Equal(t, "assigned", got.Status)

	// BK-3a：patch 写 ticket_events（action=updated）
	env.roles[env.u1] = []string{"operator"}
	env.roles[env.colleague] = []string{"operator"}
	plain := newTicketHelper(t, env.svc, env.u1, env.d1, "事件留痕工单")
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: plain.ID, Title: strPtr("改一次"),
	}, env.u1)
	require.NoError(t, err)
	var evCount int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM ticket_events WHERE ticket_id = $1 AND action = 'updated'`,
		plain.ID).Scan(&evCount))
	assert.Equal(t, 1, evCount, "patch 应写 updated 事件（BK-3 审计断档修复）")

	// BK-3b：close 后 update → 90004（条件更新消除 TOCTOU；90004 复活）
	require.NoError(t, env.svc.Close(ctx, &model.CloseTicketRequest{ID: plain.ID}, env.u1))
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{
		ID: plain.ID, Title: strPtr("关闭后再改"),
	}, env.u1)
	requireErrCode(t, err, errcode.ErrTicketAlreadyClosed.Code)
}

// newTicketWithType 以指定类型创建工单
func newTicketWithType(t *testing.T, svc *Service, actorID, orgID int64, typeCode, title string) *model.Ticket {
	t.Helper()
	tk, err := svc.Create(context.Background(), &model.CreateTicketRequest{
		TypeCode: typeCode, Title: title, OrgID: orgID,
	}, actorID)
	require.NoError(t, err, "创建工单失败：type=%s title=%s", typeCode, title)
	return tk
}

// --- 辅助 ---

func createB2Org(t *testing.T, parentID int64, code, name string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ($1, $2, $3, (SELECT path::text || '.' || $4 FROM organizations WHERE id = $3)::ltree, 3, 1, 60, false)
		RETURNING id`, code, name, parentID, code).Scan(&id))
	return id
}

func createB2User(t *testing.T, username, eno string) int64 {
	t.Helper()
	// 注意：参数化 $ + ON CONFLICT 部分索引谓词在 pgx 扩展协议下报 42P02，
	// 与 setupTicket2a 一致改用字面量（值均为测试内部生成，无注入面）
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), fmt.Sprintf(`
		INSERT INTO users (username, password, employee_no, status) VALUES ('%s', 'hash', '%s', 1)
		ON CONFLICT (employee_no) WHERE employee_no IS NOT NULL AND employee_no <> '' AND deleted_at IS NULL
			DO UPDATE SET password = EXCLUDED.password
		RETURNING id`, username, eno)).Scan(&id))
	return id
}

func bindOrgMember(t *testing.T, userID, orgID int64) {
	t.Helper()
	// 裸 INSERT（对齐 org_repo_integration_test 既有模式）；测试用户/组织唯一，无冲突面
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true)`, userID, orgID)
	require.NoError(t, err)
}

func strPtr(s string) *string { return &s }

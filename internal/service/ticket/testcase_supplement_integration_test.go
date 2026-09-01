//go:build integration

package ticket

// IW 测试补充批次：TC-4 / TC-2 / F-31④ / 双删 404 / HC1 失败路径 / BK-14 生效断言

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// ---------- TC-4：UpdateTicketType patch 语义 ----------

func TestBK18_UpdateTicketTypePatch(t *testing.T) {
	svc, _, _ := setupTicketAdmin(t) // TC-4
	ctx := context.Background()
	code := fmt.Sprintf("bk18pt_%d", time.Now().UnixNano()%1e9)

	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{
		Code: code, Name: "patch 原", Description: "原描述"})
	require.NoError(t, err)

	// patch：仅改名，description/states/transitions 保持
	up, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{Name: strPtr("patch 改名")})
	require.NoError(t, err)
	assert.Equal(t, "patch 改名", up.Name)
	assert.Equal(t, "原描述", up.Description, "未传 description 应保持")

	// patch：states/transitions 更新（合法自定义图）
	up2, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{
		Name:        strPtr("patch 状态图"),
		States:      json.RawMessage(`["open","closed"]`),
		Transitions: json.RawMessage(`{"open":["closed"]}`)})
	require.NoError(t, err)
	assert.Contains(t, string(up2.States), `"closed"`)
	assert.NotContains(t, string(up2.Transitions), `"assigned"`)

	// patch：非法 states（to 不在 states 内）→ 400
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{
		Name:        strPtr("坏图"),
		States:      json.RawMessage(`["open","closed"]`),
		Transitions: json.RawMessage(`{"open":["assigned"]}`)})
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)
}

// ---------- TC-2：元数据 service 直测 ----------

func TestTC2_MetaDirectReads(t *testing.T) {
	svc, admin, envOrgID := setupTicketAdmin(t)
	ctx := context.Background()

	tpl, err := svc.CreateTicketTemplate(ctx, &model.CreateTicketTemplateRequest{
		Code: fmt.Sprintf("tc2tpl_%d", time.Now().UnixNano()%1e9),
		Name: "TC2 模板", TypeCode: "incident", OrgID: envOrgID()}, admin)
	require.NoError(t, err)
	got, err := svc.GetTicketTemplate(ctx, tpl.Code)
	require.NoError(t, err)
	assert.Equal(t, tpl.ID, got.ID)

	a := newTicketHelper(t, svc, admin, envOrgID(), "TC2 关联 A")
	b := newTicketHelper(t, svc, admin, envOrgID(), "TC2 关联 B")
	_, err = svc.CreateRelation(ctx, &model.CreateRelationRequest{
		SourceTicketID: a.ID, TargetTicketID: b.ID, RelationType: "related"}, admin)
	require.NoError(t, err)
	rels, err := svc.ListRelations(ctx, a.ID, admin)
	require.NoError(t, err)
	assert.Len(t, rels, 1, "A 的关联列表应含 1 条")
}

// ---------- F-31④：relation 越权负向 ----------

func TestF31RelationForbidden(t *testing.T) {
	svc, _, vgA, _, ma, _, mb, _ := setupD12(t)
	ctx := context.Background()

	tkA := newTicketHelper(t, svc, ma, vgA, "F31 源单")
	tkB := newTicketHelper(t, svc, ma, vgA, "F31 目标单")
	_, err := svc.CreateRelation(ctx, &model.CreateRelationRequest{
		SourceTicketID: tkA.ID, TargetTicketID: tkB.ID, RelationType: "related"}, mb)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)
}

// ---------- 双删 404（BK-16 防回归） ----------

func TestTicketDoubleDeleteSecond404(t *testing.T) {
	svc, admin, envOrgID := setupTicketAdmin(t)
	ctx := context.Background()
	tk := newTicketHelper(t, svc, admin, envOrgID(), "双删工单")

	require.NoError(t, svc.Delete(ctx, tk.ID, admin), "首次删除应成功")
	err := svc.Delete(ctx, tk.ID, admin)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
}

// ---------- HC1 失败路径：工单不存在 → 评论/事件全不落库 ----------

func TestHC1_CommentFailNoResidue(t *testing.T) {
	svc, admin, envOrgID := setupTicketAdmin(t)
	ctx := context.Background()
	newTicketHelper(t, svc, admin, envOrgID(), "基线单")
	var commentsBefore, eventsBefore int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM ticket_comments), (SELECT COUNT(*) FROM ticket_events)`).
		Scan(&commentsBefore, &eventsBefore))

	_, err := svc.CreateComment(ctx, &model.CreateCommentRequest{
		TicketID: 999999999, Content: "不应落库"}, admin)
	require.Error(t, err)

	var commentsAfter, eventsAfter int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT (SELECT COUNT(*) FROM ticket_comments), (SELECT COUNT(*) FROM ticket_events)`).
		Scan(&commentsAfter, &eventsAfter))
	assert.Equal(t, commentsBefore, commentsAfter, "失败评论不得残留")
	assert.Equal(t, eventsBefore, eventsAfter, "失败不得产生事件")
}

// ---------- BK-14 生效断言：scope 变更 → resolver 生效 ----------

func TestBK14_ScopeChangeAffectsResolver(t *testing.T) {
	_, _, _, _, ma, _, _, _ := setupD12(t)
	ctx := context.Background()
	resolver := NewPgxScopeResolver(testPool)

	// vg_a 成员默认 assigned：ScopePaths 不含成员组织子树
	sc, err := resolver.ResolveScope(ctx, ma)
	require.NoError(t, err)
	assert.Empty(t, sc.ScopePaths, "assigned 不应扩展子树")

	// 改 group → 成员组织子树入 ScopePaths
	_, err = testPool.Exec(ctx,
		`UPDATE user_orgs SET ticket_scope = 'group' WHERE user_id = $1`, ma)
	require.NoError(t, err)
	sc, err = resolver.ResolveScope(ctx, ma)
	require.NoError(t, err)
	assert.NotEmpty(t, sc.ScopePaths, "scope=group 后成员组织子树应入 ScopePaths")

	// 改 all → 全量开关
	_, err = testPool.Exec(ctx,
		`UPDATE user_orgs SET ticket_scope = 'all' WHERE user_id = $1`, ma)
	require.NoError(t, err)
	sc, err = resolver.ResolveScope(ctx, ma)
	require.NoError(t, err)
	assert.True(t, sc.AllScope, "scope=all 应置 AllScope")
}

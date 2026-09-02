//go:build integration

package service_test

// IW1/BK-14 + BK-13.3 集成测试：成员数据范围（ticket_scope）配置面 +
// ticket_visibility 配置守卫。SSOT: phase2/00 §9 BK-14/BK-13、09-ticket §5.2。

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// IW1/BK-14：SetMemberScope 权限矩阵 + 语义
func TestBK14_SetMemberScope(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()

	// org admin 改 member scope=group ✓
	require.NoError(t, env.orgSvc.SetMemberScope(ctx,
		&model.SetMemberScopeRequest{OrgID: env.vgID, UserID: env.mem1, TicketScope: "group"}, env.admin))
	var scope string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT ticket_scope FROM user_orgs WHERE org_id = $1 AND user_id = $2`,
		env.vgID, env.mem1).Scan(&scope))
	assert.Equal(t, "group", scope)

	// org admin 改 scope=all → 403（旁路整个 L2 = 全局可见，仅全局管理员可授）
	err := env.orgSvc.SetMemberScope(ctx,
		&model.SetMemberScopeRequest{OrgID: env.vgID, UserID: env.mem1, TicketScope: "all"}, env.admin)
	requireErrCode(t, err, errcode.ErrNoPermission)

	// 全局管理员改 scope=all ✓
	require.NoError(t, env.orgSvc.SetMemberScope(ctx,
		&model.SetMemberScopeRequest{OrgID: env.vgID, UserID: env.mem1, TicketScope: "all"}, env.super))
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT ticket_scope FROM user_orgs WHERE org_id = $1 AND user_id = $2`,
		env.vgID, env.mem1).Scan(&scope))
	assert.Equal(t, "all", scope)

	// 目标非成员 → 50007
	var outsider int64
	require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO users (username, password, employee_no, status) VALUES ('bk14out_%s', 'hash', 'EBK14OUT', 1)
		RETURNING id`, uniqueSuffix())).Scan(&outsider))
	err = env.orgSvc.SetMemberScope(ctx,
		&model.SetMemberScopeRequest{OrgID: env.vgID, UserID: outsider, TicketScope: "group"}, env.admin)
	requireErrCode(t, err, errcode.ErrNotOrgMember)

	// member 调用 → 403（无成员管理权）
	err = env.orgSvc.SetMemberScope(ctx,
		&model.SetMemberScopeRequest{OrgID: env.vgID, UserID: env.mem1, TicketScope: "group"}, env.mem2)
	requireErrCode(t, err, errcode.ErrNoPermission)
}

// IW1/BK-14：AddMember 携带 ticket_scope + 重复添加不重置已配置范围
func TestBK14_AddMemberWithScope(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()

	require.NoError(t, env.orgSvc.AddMember(ctx,
		&model.OrgMemberRequest{OrgID: env.vgID, UserID: env.mem1, TicketScope: "group"}, env.admin))
	var scope string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT ticket_scope FROM user_orgs WHERE org_id = $1 AND user_id = $2`,
		env.vgID, env.mem1).Scan(&scope))
	assert.Equal(t, "group", scope, "AddMember 显式 scope 应落库")

	// 重复添加不带 scope → 不重置
	require.NoError(t, env.orgSvc.AddMember(ctx,
		&model.OrgMemberRequest{OrgID: env.vgID, UserID: env.mem1}, env.admin))
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT ticket_scope FROM user_orgs WHERE org_id = $1 AND user_id = $2`,
		env.vgID, env.mem1).Scan(&scope))
	assert.Equal(t, "group", scope, "未显式传 scope 不得重置既有配置")

	// 显式传 assigned → 覆盖
	require.NoError(t, env.orgSvc.AddMember(ctx,
		&model.OrgMemberRequest{OrgID: env.vgID, UserID: env.mem1, TicketScope: "assigned"}, env.admin))
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT ticket_scope FROM user_orgs WHERE org_id = $1 AND user_id = $2`,
		env.vgID, env.mem1).Scan(&scope))
	assert.Equal(t, "assigned", scope)
}

// BK-13.3：org update 的 ticket_visibility 配置——虚拟组 400、实体可配且落库
func TestBK13_OrgUpdateTicketVisibility(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	orgRepo := repository.NewOrgRepo(testPool)

	// 虚拟组 → 400
	vg, err := orgRepo.FindByID(ctx, env.vgID)
	require.NoError(t, err)
	_, err = env.orgSvc.Update(ctx, &model.UpdateOrgRequest{
		ID: vg.ID, Version: vg.Version, Name: vg.Name, TicketVisibility: strPtrT("project_isolated"),
	})
	requireErrCode(t, err, errcode.ErrInvalidParams)

	// 实体 → ✓ 且落库
	p, err := orgRepo.FindByID(ctx, env.pID)
	require.NoError(t, err)
	updated, err := env.orgSvc.Update(ctx, &model.UpdateOrgRequest{
		ID: p.ID, Version: p.Version, Name: p.Name, TicketVisibility: strPtrT("project_isolated"),
	})
	require.NoError(t, err)
	assert.Equal(t, "project_isolated", updated.TicketVisibility)
	var dbVal string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT ticket_visibility FROM organizations WHERE id = $1`, env.pID).Scan(&dbVal))
	assert.Equal(t, "project_isolated", dbVal)

	// 切回
	p2, err := orgRepo.FindByID(ctx, env.pID)
	require.NoError(t, err)
	_, err = env.orgSvc.Update(ctx, &model.UpdateOrgRequest{
		ID: p2.ID, Version: p2.Version, Name: p2.Name, TicketVisibility: strPtrT("entity_transparent_read"),
	})
	require.NoError(t, err)
}

func strPtrT(s string) *string { return &s }

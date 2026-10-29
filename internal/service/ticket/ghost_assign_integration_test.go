//go:build integration

// 幽灵分派校验集成验证（2026-10-29 所有者拍板「400 拒绝」）：
// 不存在用户→30002 / 禁用用户→30003 / 正常→200 语义 / 取消分派不校验。
package ticket

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestAssign_GhostAndDisabledTarget(t *testing.T) {
	svc, setupAid, _, _, stubRoles := setupTicket2a(t)
	ctx := context.Background()
	oid := childOrgID(t, "ghost")
	// assign 是主管级动作（canOperate 要求 AllScope/锚点）——actor 授 admin
	stubRoles[setupAid] = append(stubRoles[setupAid], "admin")

	ghost := int64(999999001) // 不存在的用户 id（远离真实自增段）
	var aid, disabledID int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO users (username,password,employee_no,status) VALUES ('zz_ghost_a','hash','EZA001',1) RETURNING id`).Scan(&aid))
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO users (username,password,employee_no,status) VALUES ('zz_ghost_b','hash','EZA002',0) RETURNING id`).Scan(&disabledID))

	// 对齐 authz 批：给测试用户授 org 管理角色（HasOrgManagePermission → Create
	// 归属校验通过；role_menus 绑定同款——org:member 菜单行）
	orgMgrCode := "t_uniq_orgmgr_" + t.Name()[8:20]
	var roleID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO roles (code, name, priority, is_system)
		VALUES ($1, '测试组织管理', 50, false) RETURNING id`, orgMgrCode).Scan(&roleID))
	var menuID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO menus (code, name, menu_type, path, component, icon, permission, sort_order, is_system)
		VALUES ($1, '测试组织管理按钮', 3, '', '', '', 'org:member', 99, false)
		ON CONFLICT (code) WHERE deleted_at IS NULL DO UPDATE SET permission = EXCLUDED.permission
		RETURNING id`, "t_uniq_orgmgr_btn_"+t.Name()[8:20]).Scan(&menuID))
	_, err := testPool.Exec(ctx,
		`INSERT INTO role_menus (role_id, menu_id) VALUES ($1, $2)`, roleID, menuID)
	require.NoError(t, err)
	for _, uid := range []int64{aid, disabledID} {
		_, err = testPool.Exec(ctx,
			`INSERT INTO user_roles (user_id, role_id) VALUES ($1, $2)`, uid, roleID)
		require.NoError(t, err)
	}

	tk := newTicketHelper(t, svc, setupAid, oid, "ghost-assign-it")

	t.Run("不存在用户 → 30002", func(t *testing.T) {
		err := svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &ghost}, setupAid)
		require.Error(t, err)
		var ae *errcode.Error
		require.ErrorAs(t, err, &ae)
		require.Equal(t, errcode.ErrUserNotFound.Code, ae.Code)
		require.Contains(t, ae.Message, "不存在")
		// 工单 assigned_to 未被写入（幽灵不挂单）
		var n int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM tickets WHERE id=$1 AND assigned_to IS NOT NULL`, tk.ID).Scan(&n))
		require.Equal(t, 0, n)
	})

	t.Run("禁用用户 → 30003", func(t *testing.T) {
		err := svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &disabledID}, setupAid)
		require.Error(t, err)
		var ae *errcode.Error
		require.ErrorAs(t, err, &ae)
		require.Equal(t, errcode.ErrUserDisabled.Code, ae.Code)
	})

	t.Run("正常启用用户 → 成功且处理人在位", func(t *testing.T) {
		err := svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &aid}, setupAid)
		require.NoError(t, err)
		var assignedTo int64
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT assigned_to FROM tickets WHERE id=$1`, tk.ID).Scan(&assignedTo))
		require.Equal(t, aid, assignedTo)
	})
}

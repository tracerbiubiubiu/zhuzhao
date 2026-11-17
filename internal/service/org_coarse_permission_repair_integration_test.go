//go:build integration

package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func orgRepairPermissionActor(t *testing.T, permission string) int64 {
	t.Helper()
	role := rbacTestRole(t, "org_repair_"+uniqueSuffix(), 60, false)
	actor := rbacTestUser(t, "org_repair_"+uniqueSuffix(), "ORG_REPAIR_"+uniqueSuffix(), []int64{role.ID})
	var menuID int64
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(), `INSERT INTO menus(code,name,menu_type,permission,is_system) VALUES ($1,'合成单动作权限',3,$2,false) RETURNING id`, "org_repair_"+uniqueSuffix(), permission).Scan(&menuID))
	_, err := testPool.Exec(trustedIAMTestContext(), `INSERT INTO role_menus(role_id,menu_id) VALUES ($1,$2)`, role.ID, menuID)
	require.NoError(t, err)
	return actor
}

func orgRepairState(t *testing.T, orgID int64) string {
	t.Helper()
	var state string
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(), `SELECT md5(jsonb_build_object('org',to_jsonb(o),'members',(SELECT jsonb_agg(to_jsonb(m) ORDER BY user_id) FROM user_orgs m WHERE org_id=o.id),'roles',(SELECT jsonb_agg(to_jsonb(r) ORDER BY role_id) FROM org_roles r WHERE org_id=o.id))::text) FROM organizations o WHERE id=$1`, orgID).Scan(&state))
	return state
}

func TestOrgCoarseRepair_CreatorCannotUseOtherOrganizationCommands(t *testing.T) {
	for _, command := range []string{"owners", "add_member", "remove_member", "member_role", "scope_all", "delete", "roster", "roles_read"} {
		t.Run(command, func(t *testing.T) {
			resetRBACTables(t)
			env := setupDelegation(t)
			actor := orgRepairPermissionActor(t, "org:create")
			ctx := trustedIAMTestContext()
			orgID := env.vgID
			if command == "delete" {
				code := "vg_empty_" + uniqueSuffix()
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,is_virtual,status) SELECT $1::text,'合成空组',id,(path::text || '.' || $1::text)::ltree,true,1 FROM organizations WHERE id=$2 RETURNING id`, code, env.pID).Scan(&orgID))
			}
			before := orgRepairState(t, orgID)
			var err error
			switch command {
			case "owners":
				_, err = env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: orgID, OwnerUserIDs: []int64{actor}}, actor)
			case "add_member":
				err = env.orgSvc.AddMember(ctx, &model.OrgMemberRequest{OrgID: orgID, UserID: actor, OrgMemberRole: "admin", TicketScope: "all"}, actor)
			case "remove_member":
				err = env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: orgID, UserID: env.mem1}, actor)
			case "member_role":
				err = env.orgSvc.SetMemberRole(ctx, &model.SetOrgMemberRoleRequest{OrgID: orgID, UserID: env.mem1, OrgMemberRole: "admin"}, actor)
			case "scope_all":
				err = env.orgSvc.SetMemberScope(ctx, &model.SetMemberScopeRequest{OrgID: orgID, UserID: env.mem1, TicketScope: "all"}, actor)
			case "delete":
				err = env.orgSvc.DeleteOrgDelegated(ctx, orgID, actor)
			case "roster":
				_, err = env.orgSvc.ListMembersRoster(ctx, orgID, actor, 1, 20)
			case "roles_read":
				_, err = env.orgSvc.ListOrgRoles(ctx, orgID, actor)
			}
			require.Error(t, err, "a create permission does not authorize unrelated global org commands")
			require.Equal(t, before, orgRepairState(t, orgID), "denial must not modify organization, ownership, membership, scope or role facts")
		})
	}
}

func TestOrgCoarseRepair_FunctionalPermissionsStaySeparate(t *testing.T) {
	for _, tc := range []struct {
		permission, command string
		allowed             bool
	}{
		{"org:member", "owners", true}, {"org:update", "owners", false}, {"org:delete", "owners", false},
		{"org:member", "add_member", true}, {"org:read", "add_member", false},
		{"org:update", "scope_all", false}, {"org:member", "scope_all", false},
		{"org:read", "roster", true}, {"org:member", "roster", true},
	} {
		t.Run(tc.permission+"/"+tc.command, func(t *testing.T) {
			resetRBACTables(t)
			env := setupDelegation(t)
			actor := orgRepairPermissionActor(t, tc.permission)
			ctx := trustedIAMTestContext()
			var err error
			switch tc.command {
			case "owners":
				_, err = env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{actor}}, actor)
			case "add_member":
				err = env.orgSvc.AddMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: actor, OrgMemberRole: "member"}, actor)
			case "scope_all":
				err = env.orgSvc.SetMemberScope(ctx, &model.SetMemberScopeRequest{OrgID: env.vgID, UserID: env.mem1, TicketScope: "all"}, actor)
			case "roster":
				_, err = env.orgSvc.ListMembersRoster(ctx, env.vgID, actor, 1, 20)
			}
			if tc.allowed {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

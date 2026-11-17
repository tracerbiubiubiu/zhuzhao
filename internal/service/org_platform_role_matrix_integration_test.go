//go:build integration

package service_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// Platform functional roles and local owner delegation are separate grants.
// An org_roles source does not turn a platform role into a local-only role.
func TestOrgPlatformRoleAndLocalDelegationMatrix(t *testing.T) {
	for _, tc := range []struct {
		name, permission, command string
		localOwner, paused, allow bool
	}{
		{name: "org source member grants cross-org owners", permission: "org:member", command: "owners", allow: true},
		{name: "org source member grants cross-org add", permission: "org:member", command: "add", allow: true},
		{name: "org source member grants nonall scope", permission: "org:member", command: "scope", allow: true},
		{name: "org source member never grants all scope", permission: "org:member", command: "scope_all"},
		{name: "org source member never grants deletion", permission: "org:member", command: "delete"},
		{name: "org source delete grants empty-org deletion", permission: "org:delete", command: "delete", allow: true},
		{name: "org source create never grants owners", permission: "org:create", command: "owners"},
		{name: "paused source cannot grant global members", permission: "org:member", command: "owners", paused: true},
		{name: "local owner retains own-org management", localOwner: true, command: "own_owners", allow: true},
		{name: "local owner cannot manage other branch", localOwner: true, command: "owners"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			resetRBACTables(t)
			env := setupDelegation(t)
			ctx := trustedIAMTestContext()
			code := "vg_matrix_" + uniqueSuffix()
			var target int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,is_virtual,status) VALUES($1::text,'synthetic separate branch',1,('root.' || $1::text)::ltree,true,1) RETURNING id`, code).Scan(&target))
			actor := env.owner
			if tc.localOwner {
				_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{actor}}, env.super)
				require.NoError(t, err)
			} else {
				role := rbacTestRole(t, "matrix_"+uniqueSuffix(), 60, false)
				actor = rbacTestUser(t, "matrix_"+uniqueSuffix(), "MATRIX_"+uniqueSuffix(), nil)
				var menu int64
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO menus(code,name,menu_type,permission,is_system) VALUES($1,'synthetic exact function',3,$2,false) RETURNING id`, "matrix_"+uniqueSuffix(), tc.permission).Scan(&menu))
				_, err := testPool.Exec(ctx, `INSERT INTO role_menus(role_id,menu_id) VALUES($1,$2)`, role.ID, menu)
				require.NoError(t, err)
				require.NoError(t, env.orgSvc.BindOrgRole(ctx, &model.BindOrgRoleRequest{OrgID: env.pID, RoleID: role.ID}, env.super))
				_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,org_member_role) VALUES($1,$2,'member')`, actor, env.pID)
				require.NoError(t, err)
				var direct int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM user_roles WHERE user_id=$1`, actor).Scan(&direct))
				require.Zero(t, direct, "the tested platform grant comes only from org_roles")
				if tc.paused {
					_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, env.pID)
					require.NoError(t, err)
				}
			}
			if tc.command == "scope" || tc.command == "scope_all" {
				_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,org_member_role,ticket_scope) VALUES($1,$2,'member','assigned')`, env.mem1, target)
				require.NoError(t, err)
			}
			if tc.command == "own_owners" {
				target = env.vgID
			}
			before := orgRepairState(t, target)
			var err error
			switch tc.command {
			case "owners":
				_, err = env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: target, OwnerUserIDs: []int64{actor}}, actor)
			case "own_owners":
				_, err = env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: target, OwnerUserIDs: []int64{actor, env.mem2}}, actor)
			case "add":
				err = env.orgSvc.AddMember(ctx, &model.OrgMemberRequest{OrgID: target, UserID: env.mem1, OrgMemberRole: "member"}, actor)
			case "scope", "scope_all":
				scope := "org"
				if tc.command == "scope_all" {
					scope = "all"
				}
				err = env.orgSvc.SetMemberScope(ctx, &model.SetMemberScopeRequest{OrgID: target, UserID: env.mem1, TicketScope: scope}, actor)
			case "delete":
				err = env.orgSvc.DeleteOrgDelegated(ctx, target, actor)
			}
			if tc.allow {
				require.NoError(t, err)
				switch tc.command {
				case "owners", "own_owners":
					var owners []int64
					require.NoError(t, testPool.QueryRow(ctx, `SELECT owner_user_ids FROM organizations WHERE id=$1`, target).Scan(&owners))
					require.Contains(t, owners, actor)
					if tc.command == "own_owners" {
						require.Contains(t, owners, env.mem2)
						require.Len(t, owners, 2)
					}
				case "add":
					var count int
					require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM user_orgs WHERE org_id=$1 AND user_id=$2 AND org_member_role='member'`, target, env.mem1).Scan(&count))
					require.Equal(t, 1, count)
				case "scope":
					var scope string
					require.NoError(t, testPool.QueryRow(ctx, `SELECT ticket_scope FROM user_orgs WHERE org_id=$1 AND user_id=$2`, target, env.mem1).Scan(&scope))
					require.Equal(t, "org", scope)
				case "delete":
					var deleted bool
					require.NoError(t, testPool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL FROM organizations WHERE id=$1`, target).Scan(&deleted))
					require.True(t, deleted)
				}
			} else {
				require.Error(t, err)
				require.Equal(t, before, orgRepairState(t, target), "denial leaves target organization and members untouched")
			}
		})
	}
}

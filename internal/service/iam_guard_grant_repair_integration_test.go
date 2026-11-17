//go:build integration

package service_test

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func grantOrgService(t *testing.T) *service.OrgService {
	t.Helper()
	users, roles := repository.NewUserRepo(testPool), repository.NewRoleRepo(testPool)
	return service.NewOrgService(repository.NewOrgRepo(testPool), users, service.NewOrgDelegationService(testPool), service.NewRBACService(roles, users, repository.NewMenuRepo(testPool), rbacTestEnforcer(t)))
}

func TestIAMGuard_NewMembershipCannotGrantStrongerRole(t *testing.T) {
	for _, command := range []string{"create", "set_orgs", "add_member", "set_owners"} {
		for _, latent := range []string{"active", "disabled_role", "disabled_org", "disabled_ancestor"} {
			t.Run(command+"/"+latent, func(t *testing.T) {
				f := targetProtectionSetup(t)
				ctx := trustedIAMTestContext()
				_, err := testPool.Exec(ctx, `INSERT INTO menus(code,name,menu_type,permission) VALUES('grant_org_member','合成成员动作',3,'org:member')`)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, `INSERT INTO role_menus(role_id,menu_id) SELECT ur.role_id,m.id FROM user_roles ur,menus m WHERE ur.user_id=$1 AND m.code='grant_org_member'`, f.actor)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1`, f.target)
				require.NoError(t, err)
				switch latent {
				case "disabled_role":
					_, err = testPool.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, f.strong.ID)
				case "disabled_org":
					_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, f.org)
				case "disabled_ancestor":
					_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, f.parent)
				}
				require.NoError(t, err)
				before := targetProtectionSnapshot(t, f.target)
				switch command {
				case "create":
					_, err = f.svc.Create(ctx, &model.CreateUserRequest{Username: "grant_new", EmployeeNo: "GRANT_NEW", Password: "SyntheticKnownPassword#2026", RoleIDs: []int64{f.weak.ID}, OrgIDs: []int64{f.org}}, f.actor)
				case "set_orgs":
					err = f.svc.SetUserOrgs(ctx, &model.SetUserOrgsRequest{UserID: f.target, OrgIDs: []int64{f.org}}, f.actor)
				case "add_member":
					err = grantOrgService(t).AddMember(ctx, &model.OrgMemberRequest{OrgID: f.org, UserID: f.target}, f.actor)
				case "set_owners":
					_, err = grantOrgService(t).SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: f.org, OwnerUserIDs: []int64{f.target}}, f.actor)
				}
				require.ErrorIs(t, err, errcode.ErrCannotAssignHigherRole, "new membership must check potential bindings even while source disabled")
				require.Equal(t, before, targetProtectionSnapshot(t, f.target))
				var created int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM users WHERE employee_no='GRANT_NEW'`).Scan(&created))
				require.Zero(t, created)
			})
		}
	}
}

func TestIAMGuard_MembershipWithoutNewGrantPreservesManagement(t *testing.T) {
	for _, command := range []string{"add_member", "set_owners", "retained_expired"} {
		t.Run(command, func(t *testing.T) {
			resetRBACTables(t)
			env := setupDelegation(t)
			ctx := trustedIAMTestContext()
			_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
			require.NoError(t, err)
			if command == "retained_expired" {
				role := rbacTestRole(t, "grant_retained", 5, false)
				weak := rbacTestRole(t, "grant_retained_direct", 80, false)
				require.NoError(t, repository.NewUserRepo(testPool).SetRoles(ctx, env.mem1, []int64{weak.ID}))
				_, err := testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2);`, env.vgID, role.ID)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, `UPDATE user_orgs SET expires_at=NOW()-INTERVAL '1 hour',source='hr' WHERE user_id=$1 AND org_id=$2`, env.mem1, env.vgID)
				require.NoError(t, err)
				svc, _, _ := newUserServiceForD2(t)
				require.NoError(t, svc.SetUserOrgs(ctx, &model.SetUserOrgsRequest{UserID: env.mem1, OrgIDs: []int64{env.vgID}}, env.owner))
				var expired bool
				require.NoError(t, testPool.QueryRow(ctx, `SELECT expires_at<NOW() FROM user_orgs WHERE user_id=$1 AND org_id=$2`, env.mem1, env.vgID).Scan(&expired))
				require.True(t, expired)
				return
			}
			user := rbacTestUser(t, "grant_plain_"+command, fmt.Sprintf("GRANT_PLAIN_%s", command), nil)
			if command == "add_member" {
				require.NoError(t, env.orgSvc.AddMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: user}, env.admin))
			} else {
				_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner, user}}, env.owner)
				require.NoError(t, err)
			}
		})
	}
}

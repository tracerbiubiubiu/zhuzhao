//go:build integration

package service_test

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

func iambOrg(t *testing.T, suffix string) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(trustedIAMTestContext(), `INSERT INTO organizations(code,name,parent_id,path,status) VALUES($1,'合成组织',1,$2::ltree,1) RETURNING id`, suffix, "root."+suffix).Scan(&id))
	return id
}

func TestIAMB_AncestorSuspendsOrganizationSources(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := trustedIAMTestContext()
	roles := repository.NewRoleRepo(testPool)
	orgs := repository.NewOrgRepo(testPool)
	grant := rbacTestRole(t, "iamb_org_grant", 50, false)
	_, err := testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2)`, env.vgID, grant.ID)
	require.NoError(t, err)
	var menuID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO menus(code,name,menu_type,permission) VALUES('iamb_org_manage','合成组织动作',3,'org:member') RETURNING id`).Scan(&menuID))
	_, err = testPool.Exec(ctx, `INSERT INTO role_menus(role_id,menu_id) VALUES($1,$2)`, grant.ID, menuID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE user_orgs SET ticket_scope='all' WHERE user_id=$1 AND org_id=$2`, env.admin, env.vgID)
	require.NoError(t, err)
	outside := iambOrg(t, "iamb_outside_"+uniqueSuffix())
	_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,ticket_scope) VALUES($1,$2,'group')`, env.admin, outside)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=ARRAY[$1::bigint] WHERE id=$2`, env.admin, env.pID)
	require.NoError(t, err)
	resolver := ticketservice.NewPgxScopeResolver(testPool)
	codes, err := roles.GetEffectiveRoleCodes(ctx, env.admin)
	require.NoError(t, err)
	require.Contains(t, codes, grant.Code)
	scope, err := resolver.ResolveScope(ctx, env.admin)
	require.NoError(t, err)
	require.True(t, scope.AllScope)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, env.pID)
	require.NoError(t, err)
	codes, err = roles.GetEffectiveRoleCodes(ctx, env.admin)
	require.NoError(t, err)
	assert.NotContains(t, codes, grant.Code, "paused ancestor must suspend the descendant org source")
	assert.Contains(t, codes, "viewer", "independent direct role must remain")
	ids, err := roles.ListRoleIDsByUserID(ctx, env.admin)
	require.NoError(t, err)
	assert.NotContains(t, ids, grant.ID, "menu projection must use the same source")
	global, err := env.deleg.HasOrgActionPermission(ctx, env.admin, "org:member")
	require.NoError(t, err)
	assert.False(t, global, "fallback org management must not preserve paused grant")
	scope, err = resolver.ResolveScope(ctx, env.admin)
	require.NoError(t, err)
	assert.False(t, scope.AllScope)
	assert.Len(t, scope.ScopePaths, 1, "unrelated group source must remain")
	var outsidePath string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT path::text FROM organizations WHERE id=$1`, outside).Scan(&outsidePath))
	assert.Equal(t, []string{outsidePath}, scope.ScopePaths)
	assert.Equal(t, []string{outsidePath}, scope.AnchorPaths, "paused nearest entity must not fall back to root")
	member, err := orgs.IsMember(ctx, env.vgID, env.admin)
	require.NoError(t, err)
	assert.False(t, member)
	branch, err := orgs.IsInOrgBranch(ctx, env.vgID, env.admin)
	require.NoError(t, err)
	assert.False(t, branch)
	assignable, err := repository.NewAssignmentRuleRepo(testPool).UserBelongsToOrg(ctx, env.admin, env.vgID)
	require.NoError(t, err)
	assert.False(t, assignable)
	p, err := env.deleg.EffectiveOrgPriority(ctx, env.admin, env.vgID)
	require.NoError(t, err)
	assert.Equal(t, service.OrgRoleMemberPriority, p)
	delegated, err := env.deleg.IsOrgAdminOrOwner(ctx, env.admin, env.vgID)
	require.NoError(t, err)
	assert.False(t, delegated)
	var vgPath string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT path::text FROM organizations WHERE id=$1`, env.vgID).Scan(&vgPath))
	ancestor, err := env.deleg.IsAncestorOwner(ctx, env.admin, env.vgID, vgPath)
	require.NoError(t, err)
	assert.False(t, ancestor)
	// Restore only after revoking/expiring sources: re-enable cannot revive them.
	_, err = testPool.Exec(ctx, `DELETE FROM org_roles WHERE org_id=$1`, env.vgID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE user_orgs SET expires_at=NOW()-INTERVAL '1 second' WHERE user_id=$1 AND org_id=$2`, env.admin, env.vgID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET status=1,owner_user_ids='{}' WHERE id=$1`, env.pID)
	require.NoError(t, err)
	codes, err = roles.GetEffectiveRoleCodes(ctx, env.admin)
	require.NoError(t, err)
	assert.NotContains(t, codes, grant.Code)
	scope, err = resolver.ResolveScope(ctx, env.admin)
	require.NoError(t, err)
	assert.False(t, scope.AllScope)
}

func TestIAMB_DisabledRoleParentStopsMenuClosure(t *testing.T) {
	resetRBACTables(t)
	ctx := trustedIAMTestContext()
	child := rbacTestRole(t, "iamb_child", 20, false)
	parent := rbacTestRole(t, "iamb_parent", 30, false)
	grand := rbacTestRole(t, "iamb_grand", 40, false)
	user := rbacTestUser(t, "iamb_parent_user", "IAMB_PARENT", []int64{child.ID, grand.ID})
	_, err := testPool.Exec(ctx, `UPDATE roles SET parent_id=$2 WHERE id=$1`, child.ID, parent.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE roles SET parent_id=$2,status=0 WHERE id=$1`, parent.ID, grand.ID)
	require.NoError(t, err)
	repo := repository.NewRoleRepo(testPool)
	ids, err := repo.ListRoleIDsByUserID(ctx, user)
	require.NoError(t, err)
	assert.ElementsMatch(t, []int64{child.ID, grand.ID}, ids)
	codes, err := repo.GetEffectiveRoleCodes(ctx, user)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{child.Code, grand.Code}, codes)
}

func TestIAMB_MembershipDeltaPreservesFacts(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := trustedIAMTestContext()
	repo := repository.NewOrgRepo(testPool)
	_, err := env.orgSvc.SetOwners(ctx, &model.SetOrgOwnersRequest{OrgID: env.vgID, OwnerUserIDs: []int64{env.owner}}, env.super)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE user_orgs SET is_primary=true,ticket_scope='group',source='local',expires_at='2020-01-03',joined_at='2020-01-02' WHERE user_id=$1 AND org_id=$2`, env.owner, env.vgID)
	require.NoError(t, err)
	var originalExpiry, originalJoined time.Time
	require.NoError(t, testPool.QueryRow(ctx, `SELECT expires_at,joined_at FROM user_orgs WHERE user_id=$1 AND org_id=$2`, env.owner, env.vgID).Scan(&originalExpiry, &originalJoined))
	require.NoError(t, repo.SetUserOrgs(ctx, env.owner, []int64{env.vgID, env.vgID}, nil))
	var role, scope, source string
	var expiry *time.Time
	var joined time.Time
	var primary bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT org_member_role,ticket_scope,source,expires_at,joined_at,is_primary FROM user_orgs WHERE user_id=$1 AND org_id=$2`, env.owner, env.vgID).Scan(&role, &scope, &source, &expiry, &joined, &primary))
	assert.Equal(t, "owner", role)
	assert.Equal(t, "group", scope)
	assert.Equal(t, "local", source)
	require.NotNil(t, expiry, "same-set save must not erase an expired membership and revive it")
	assert.True(t, expiry.Equal(originalExpiry), "expiry must be preserved exactly")
	assert.True(t, joined.Equal(originalJoined), "joined_at must be preserved exactly")
	assert.False(t, primary, "nil/null preserves old clear-primary wire")
	added := iambOrg(t, "iamb_added_"+uniqueSuffix())
	_, err = testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=ARRAY[$1::bigint] WHERE id=$2`, env.owner, added)
	require.NoError(t, err)
	require.NoError(t, repo.SetUserOrgs(ctx, env.owner, []int64{env.vgID, added}, &added))
	var addedExpiry *time.Time
	require.NoError(t, testPool.QueryRow(ctx, `SELECT org_member_role,ticket_scope,source,expires_at,is_primary FROM user_orgs WHERE user_id=$1 AND org_id=$2`, env.owner, added).Scan(&role, &scope, &source, &addedExpiry, &primary))
	assert.Equal(t, "member", role)
	assert.Equal(t, "assigned", scope)
	assert.Equal(t, "local", source)
	assert.Nil(t, addedExpiry)
	assert.True(t, primary)
	var owners []int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT owner_user_ids FROM organizations WHERE id=$1`, added).Scan(&owners))
	assert.Contains(t, owners, env.owner, "newly requested org's owner-only fact must remain")
	require.NoError(t, repo.SetUserOrgs(ctx, env.owner, []int64{added}, nil))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT owner_user_ids FROM organizations WHERE id=$1`, env.vgID).Scan(&owners))
	assert.NotContains(t, owners, env.owner)
	// An invalid added organization must roll back membership and owner cleanup.
	require.Error(t, repo.SetUserOrgs(ctx, env.owner, []int64{999999999}, nil))
	rows, err := repo.GetUserOrgs(ctx, env.owner)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, added, rows[0].OrgID)
}

func TestIAMB_DelegatePauseKeepsBusinessAndArchivePaths(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := trustedIAMTestContext()
	_, err := testPool.Exec(ctx, `INSERT INTO ticket_types(code,name) VALUES('iamb_type','合成类型') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	var ticketID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,priority,status,created_by,org_id,org_path) SELECT 'iamb_type','合成停用测试',3,'closed',$1,id,path FROM organizations WHERE id=$2 RETURNING id`, env.mem1, env.vgID).Scan(&ticketID))
	resourceGate := ticketservice.NewResource(repository.NewTicketRepo(testPool), ticketservice.NewPgxScopeResolver(testPool), env.deleg, nil)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET deleted_at=NOW() WHERE id=$1`, env.vgID)
	require.NoError(t, err)
	ok, err := resourceGate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.admin, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(ticketID), Action: "read"})
	require.NoError(t, err)
	assert.True(t, ok, "BK20 archive delegation is not changed by status-only pause")
	_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, env.pID)
	require.NoError(t, err)
	ok, err = resourceGate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.admin, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(ticketID), Action: "read"})
	require.NoError(t, err)
	assert.False(t, ok)
	filter, err := resourceGate.GetFilter(ctx, env.admin, "list")
	require.NoError(t, err)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM tickets WHERE ("+filter.Where+") AND id=$3", append(filter.Args, ticketID)...).Scan(&count))
	assert.Zero(t, count, "list/total/export filter must match detail")
	ok, err = resourceGate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.mem1, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(ticketID), Action: "read"})
	require.NoError(t, err)
	assert.True(t, ok, "independent creator relation remains")
}

func TestIAMB_TargetProtectionProjectionKeepsDisabledAccount(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := trustedIAMTestContext()
	grant := rbacTestRole(t, "iamb_target_org", 5, false)
	parent := rbacTestRole(t, "iamb_target_parent", 10, false)
	_, err := testPool.Exec(ctx, `UPDATE roles SET parent_id=$2 WHERE id=$1`, grant.ID, parent.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2)`, env.vgID, grant.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, env.admin)
	require.NoError(t, err)
	repo := repository.NewRoleRepo(testPool)
	targetRoles, err := repo.GetEffectiveRolesForTargetProtection(ctx, env.admin)
	require.NoError(t, err)
	var targetIDs []int64
	priorities := map[string]int{}
	for _, r := range targetRoles {
		targetIDs = append(targetIDs, r.ID)
		priorities[r.Code] = r.Priority
	}
	ids, err := repo.ListRoleIDsByUserID(ctx, env.admin)
	require.NoError(t, err)
	assert.ElementsMatch(t, ids, targetIDs)
	assert.Equal(t, 5, priorities[grant.Code])
	assert.Equal(t, 10, priorities[parent.Code])
	assert.Equal(t, 40, priorities["viewer"])
	_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, env.pID)
	require.NoError(t, err)
	targetRoles, err = repo.GetEffectiveRolesForTargetProtection(ctx, env.admin)
	require.NoError(t, err)
	require.Len(t, targetRoles, 1)
	assert.Equal(t, "viewer", targetRoles[0].Code, "actor direct and target effective projections remain separate")
}

func TestIAMB_ManagedMembershipChangesAreHeld(t *testing.T) {
	for _, source := range []string{"hr", "system", "unknown"} {
		t.Run(source, func(t *testing.T) {
			resetRBACTables(t)
			env := setupDelegation(t)
			ctx := trustedIAMTestContext()
			repo := repository.NewOrgRepo(testPool)
			_, err := testPool.Exec(ctx, `UPDATE user_orgs SET source=$3,ticket_scope='group',expires_at='2020-01-03',joined_at='2020-01-02' WHERE user_id=$1 AND org_id=$2`, env.admin, env.vgID, source)
			require.NoError(t, err)
			require.NoError(t, repo.SetUserOrgs(ctx, env.admin, []int64{env.vgID}, nil), "unchanged nonlocal fact may be retained")
			require.Error(t, repo.SetUserOrgs(ctx, env.admin, nil, nil), "local replacement cannot silently delete unbound managed/unknown source")
			require.Error(t, repo.SetUserOrgs(ctx, env.admin, []int64{env.vgID}, &env.vgID), "local replacement cannot rewrite a managed primary")
			var gotSource, scope string
			var expiry *time.Time
			var primary bool
			require.NoError(t, testPool.QueryRow(ctx, `SELECT source,ticket_scope,expires_at,is_primary FROM user_orgs WHERE user_id=$1 AND org_id=$2`, env.admin, env.vgID).Scan(&gotSource, &scope, &expiry, &primary))
			assert.Equal(t, source, gotSource)
			assert.Equal(t, "group", scope)
			require.NotNil(t, expiry)
			assert.Equal(t, 2020, expiry.Year())
			assert.False(t, primary)
		})
	}
}

func TestIAMB_PrimaryEmptyAndOwnerCleanupCompatibility(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := trustedIAMTestContext()
	repo := repository.NewOrgRepo(testPool)
	added := iambOrg(t, "iamb_primary_"+uniqueSuffix())
	_, err := testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=ARRAY[$1::bigint] WHERE id=$2`, env.admin, env.pID)
	require.NoError(t, err)
	require.NoError(t, repo.SetUserOrgs(ctx, env.admin, []int64{env.vgID, added, added}, &added))
	rows, err := repo.GetUserOrgs(ctx, env.admin)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, added, rows[0].OrgID)
	assert.True(t, rows[0].IsPrimary)
	var owners []int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT owner_user_ids FROM organizations WHERE id=$1`, env.pID).Scan(&owners))
	assert.NotContains(t, owners, env.admin, "orphan owner-only references outside requested set must also be cleared")
	require.Error(t, repo.SetUserOrgs(ctx, env.admin, []int64{env.vgID}, &added))
	rows, err = repo.GetUserOrgs(ctx, env.admin)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	require.NoError(t, repo.SetUserOrgs(ctx, env.admin, []int64{env.vgID}, nil))
	rows, err = repo.GetUserOrgs(ctx, env.admin)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.False(t, rows[0].IsPrimary)
	require.NoError(t, repo.SetUserOrgs(ctx, env.admin, []int64{}, nil))
	rows, err = repo.GetUserOrgs(ctx, env.admin)
	require.NoError(t, err)
	assert.Empty(t, rows)
	// Invalid old facts are held; saving is not a hidden historical cleanup path.
	_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, env.admin, env.vgID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET deleted_at=NOW() WHERE id=$1`, env.vgID)
	require.NoError(t, err)
	require.Error(t, repo.SetUserOrgs(ctx, env.admin, []int64{}, nil))
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM user_orgs WHERE user_id=$1`, env.admin).Scan(&count))
	assert.Equal(t, 1, count)
}

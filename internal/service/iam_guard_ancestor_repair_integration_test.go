//go:build integration

package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestIAMGuard_OrganizationPathReuseUsesParentIDs(t *testing.T) {
	for _, oldStatus := range []int{0, 1} {
		t.Run(string(rune('0'+oldStatus)), func(t *testing.T) {
			resetRBACTables(t)
			env := setupDelegation(t)
			ctx := context.Background()
			code := "iam_reuse_" + uniqueSuffix()
			var tombstone, parent, child int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status,deleted_at) VALUES($1,'合成旧组织',1,$2::ltree,$3,NOW()) RETURNING id`, code, "root."+code, oldStatus).Scan(&tombstone))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status) VALUES($1,'合成新组织',1,$2::ltree,1) RETURNING id`, code, "root."+code).Scan(&parent))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status,owner_user_ids) VALUES($1,'合成新子组',$2,$3::ltree,1,ARRAY[$4::bigint]) RETURNING id`, code+"_child", parent, "root."+code+".child", env.owner).Scan(&child))
			role := rbacTestRole(t, "reuse_grant_"+uniqueSuffix(), 50, false)
			_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2);`, env.mem1, child)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2)`, child, role.ID)
			require.NoError(t, err)
			roles := repository.NewRoleRepo(testPool)
			codes, err := roles.GetEffectiveRoleCodes(ctx, env.mem1)
			require.NoError(t, err)
			require.Contains(t, codes, role.Code, "unrelated tombstone with reused path is not this live org's ancestor")
			member, err := repository.NewOrgRepo(testPool).IsMember(ctx, child, env.mem1)
			require.NoError(t, err)
			require.True(t, member)
			priority, err := env.deleg.EffectiveOrgPriority(ctx, env.owner, child)
			require.NoError(t, err)
			require.Equal(t, service.OrgRoleOwnerPriority, priority, "BK20 status evaluation must follow actual IDs too")
			_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, parent)
			require.NoError(t, err)
			codes, err = roles.GetEffectiveRoleCodes(ctx, env.mem1)
			require.NoError(t, err)
			require.NotContains(t, codes, role.Code, "real parent status still suspends the source")
			_ = tombstone
		})
	}
}

func TestIAMGuard_OrganizationCycleContributesNoSource(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `UPDATE organizations SET parent_id=$1 WHERE id=$2`, env.vgID, env.pID)
	require.NoError(t, err)
	role := rbacTestRole(t, "cycle_grant", 5, false)
	_, err = testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2)`, env.vgID, role.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=ARRAY[$1::bigint] WHERE id=$2`, env.owner, env.vgID)
	require.NoError(t, err)
	codes, err := repository.NewRoleRepo(testPool).GetEffectiveRoleCodes(ctx, env.mem1)
	require.NoError(t, err)
	require.NotContains(t, codes, role.Code)
	member, err := repository.NewOrgRepo(testPool).IsMember(ctx, env.vgID, env.mem1)
	require.NoError(t, err)
	require.False(t, member)
	priority, err := env.deleg.EffectiveOrgPriority(ctx, env.owner, env.vgID)
	require.NoError(t, err)
	require.Equal(t, service.OrgRoleMemberPriority, priority)
}

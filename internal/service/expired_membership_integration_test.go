//go:build integration

package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// 过期成员行保留用于管理，不再是授权凭据。显式 owner_user_ids 是独立权源。
func TestExpiredMembership_AuthorizationBoundaries(t *testing.T) {
	cases := []struct {
		name, expires string
		active        bool
	}{
		{"permanent", "NULL", true},
		{"future", "NOW()+INTERVAL '1 hour'", true},
		{"past", "NOW()-INTERVAL '1 minute'", false},
		{"boundary", "NOW()", false},
	}
	for _, role := range []string{"admin", "owner"} {
		for _, tc := range cases {
			t.Run(role+"/"+tc.name, func(t *testing.T) {
				ctx := context.Background()
				env := setupDelegation(t)
				_, err := testPool.Exec(ctx, `UPDATE user_orgs SET org_member_role=$3, expires_at=`+tc.expires+` WHERE org_id=$1 AND user_id=$2`, env.vgID, env.admin, role)
				require.NoError(t, err)
				expectedPriority := service.OrgRoleMemberPriority
				if tc.active {
					expectedPriority = service.OrgRoleAdminPriority
					if role == "owner" {
						expectedPriority = service.OrgRoleOwnerPriority
					}
				}
				priority, err := env.deleg.EffectiveOrgPriority(ctx, env.admin, env.vgID)
				require.NoError(t, err)
				assert.Equal(t, expectedPriority, priority)
				delegated, err := env.deleg.IsOrgAdminOrOwner(ctx, env.admin, env.vgID)
				require.NoError(t, err)
				assert.Equal(t, tc.active, delegated)
				orgs := repository.NewOrgRepo(testPool)
				member, err := orgs.IsMember(ctx, env.vgID, env.admin)
				require.NoError(t, err)
				assert.Equal(t, tc.active, member)
				branch, err := orgs.IsInOrgBranch(ctx, env.vgID, env.admin)
				require.NoError(t, err)
				assert.Equal(t, tc.active, branch, "建单归属不接受过期成员关系")
				target, err := repository.NewAssignmentRuleRepo(testPool).UserBelongsToOrg(ctx, env.admin, env.vgID)
				require.NoError(t, err)
				assert.Equal(t, tc.active, target, "规则写侧不接受过期目标成员")
				_, rosterErr := env.orgSvc.ListMembersRoster(ctx, env.vgID, env.admin, 1, 20)
				if tc.active {
					require.NoError(t, rosterErr)
				} else {
					require.ErrorIs(t, rosterErr, errcode.ErrNoPermission)
				}
				checkMembershipTicketAccess(t, env, tc.active)
				removeErr := env.orgSvc.RemoveMember(ctx, &model.OrgMemberRequest{OrgID: env.vgID, UserID: env.mem2}, env.admin)
				if tc.active {
					require.NoError(t, removeErr)
				} else {
					require.ErrorIs(t, removeErr, errcode.ErrNoPermission)
				}
			})
		}
	}
}

func checkMembershipTicketAccess(t *testing.T, env *dEnv, expected bool) {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `INSERT INTO ticket_types(code,name) VALUES('incident','membership regression') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	var id int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,priority,status,created_by,org_id,org_path) SELECT 'incident','membership regression',3,'open',$1,id,path FROM organizations WHERE id=$2 RETURNING id`, env.mem1, env.vgID).Scan(&id))
	r := ticketservice.NewResource(repository.NewTicketRepo(testPool), ticketservice.NewPgxScopeResolver(testPool), env.deleg, nil)
	for _, action := range []string{"read", "update", "delete"} {
		ok, e := r.Authorize(ctx, resource.AuthorizeRequest{UserID: env.admin, Roles: []string{"operator"}, ResourceID: fmt.Sprint(id), Action: action})
		require.NoError(t, e)
		assert.Equal(t, expected, ok, action)
	}
	f, err := r.GetFilter(ctx, env.admin, "list")
	require.NoError(t, err)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM tickets WHERE "+f.Where, f.Args...).Scan(&count))
	if expected {
		assert.Equal(t, 1, count)
	} else {
		assert.Zero(t, count)
	}
}

func TestExpiredMembership_ExplicitOwnerRemainsIndependent(t *testing.T) {
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `UPDATE user_orgs SET expires_at=NOW()-INTERVAL '1 minute' WHERE org_id=$1 AND user_id=$2`, env.vgID, env.admin)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=ARRAY[$1::bigint] WHERE id=$2`, env.admin, env.vgID)
	require.NoError(t, err)
	p, err := env.deleg.EffectiveOrgPriority(ctx, env.admin, env.vgID)
	require.NoError(t, err)
	assert.Equal(t, service.OrgRoleOwnerPriority, p)
	checkMembershipTicketAccess(t, env, true)
}

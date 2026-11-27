//go:build integration

package service_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	ticketservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

func identityTicket(t *testing.T, orgID, creator int64) int64 {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `INSERT INTO ticket_types(code,name) VALUES('iam_identity','Synthetic identity') ON CONFLICT DO NOTHING`)
	require.NoError(t, err)
	var id int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path) SELECT 'iam_identity','Synthetic identity','closed',$1,id,path FROM organizations WHERE id=$2 RETURNING id`, creator, orgID).Scan(&id))
	return id
}
func identityFilterCount(t *testing.T, gate *ticketservice.Resource, userID, ticketID int64) int {
	t.Helper()
	ctx := context.Background()
	f, err := gate.GetFilter(ctx, userID, "list")
	require.NoError(t, err)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM tickets WHERE ("+f.Where+") AND id=$3", append(f.Args, ticketID)...).Scan(&count))
	return count
}
func identityRecreate(t *testing.T, old int64, virtual bool) int64 {
	t.Helper()
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `INSERT INTO organizations(code,name,parent_id,path,is_virtual,status) SELECT code,'Synthetic recreated',parent_id,path,$2,1 FROM organizations WHERE id=$1 RETURNING id`, old, virtual).Scan(&id))
	return id
}

func TestIAMGuard_OldOwnerCannotDelegateThroughReusedPath(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=ARRAY[$1::bigint],deleted_at=NOW() WHERE id=$2`, env.owner, env.vgID)
	require.NoError(t, err)
	recreated := identityRecreate(t, env.vgID, false)
	var child int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status) SELECT $1::text,'Synthetic child',id,(path::text||'.child')::ltree,1 FROM organizations WHERE id=$2 RETURNING id`, "identity_child_"+uniqueSuffix(), recreated).Scan(&child))
	oldTicket, newTicket := identityTicket(t, env.vgID, env.mem1), identityTicket(t, child, env.mem1)
	var path string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT path::text FROM organizations WHERE id=$1`, child).Scan(&path))
	inherited, err := env.deleg.IsAncestorOwner(ctx, env.owner, child, path)
	require.NoError(t, err)
	assert.False(t, inherited, "historical same-path owner is not an ancestor by identity")
	gate := ticketservice.NewResource(repository.NewTicketRepo(testPool), ticketservice.NewPgxScopeResolver(testPool), env.deleg)
	assert.Zero(t, identityFilterCount(t, gate, env.owner, newTicket))
	require.Equal(t, 1, identityFilterCount(t, gate, env.owner, oldTicket), "BK20 actual old org_id delegation remains")
	for _, tc := range []struct {
		id   int64
		want bool
	}{{newTicket, false}, {oldTicket, true}} {
		ok, err := gate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.owner, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(tc.id), Action: "read"})
		require.NoError(t, err)
		assert.Equal(t, tc.want, ok)
	}
}

func TestIAMGuard_NearestEntityAnchorUsesActualParentIDs(t *testing.T) {
	for _, oldTransparent := range []bool{true, false} {
		t.Run(fmt.Sprint(oldTransparent), func(t *testing.T) {
			resetRBACTables(t)
			ctx := context.Background()
			var original string
			require.NoError(t, testPool.QueryRow(ctx, `SELECT ticket_visibility FROM organizations WHERE id=1`).Scan(&original))
			defer func() {
				_, err := testPool.Exec(ctx, `UPDATE organizations SET ticket_visibility=$1 WHERE id=1`, original)
				require.NoError(t, err)
			}()
			rootVisibility, oldVisibility := "project_isolated", "entity_transparent_read"
			if !oldTransparent {
				rootVisibility, oldVisibility = oldVisibility, rootVisibility
			}
			_, err := testPool.Exec(ctx, `UPDATE organizations SET ticket_visibility=$1 WHERE id=1`, rootVisibility)
			require.NoError(t, err)
			code := "vg_identity_" + uniqueSuffix()
			var old int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,is_virtual,status,ticket_visibility,deleted_at) VALUES($1,'Synthetic old entity',1,$2::ltree,false,1,$3,NOW()) RETURNING id`, code, "root."+code, oldVisibility).Scan(&old))
			current := identityRecreate(t, old, true)
			viewer := rbacTestRole(t, "identity_viewer", 40, false)
			user := rbacTestUser(t, "identity_member", "IDENTITY_MEMBER", []int64{viewer.ID})
			_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, user, current)
			require.NoError(t, err)
			scope, err := ticketservice.NewPgxScopeResolver(testPool).ResolveScope(ctx, user)
			require.NoError(t, err)
			if oldTransparent {
				require.Empty(t, scope.AnchorPaths, "deleted same-path entity cannot supply transparent visibility")
			} else {
				require.Equal(t, []string{"root"}, scope.AnchorPaths, "deleted entity cannot hide the actual root anchor")
			}
		})
	}
}

func TestIAMGuard_LivePathScopeCannotReadOrAssignHistoricalIdentity(t *testing.T) {
	for _, kind := range []string{"transparent", "group"} {
		t.Run(kind, func(t *testing.T) {
			resetRBACTables(t)
			env := setupDelegation(t)
			ctx := context.Background()
			code := "identity_scope_" + uniqueSuffix()
			var old int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status,deleted_at) VALUES($1,'Synthetic historical',1,$2::ltree,1,NOW()) RETURNING id`, code, "root."+code).Scan(&old))
			current := identityRecreate(t, old, false)
			ticketScope := "assigned"
			if kind == "group" {
				ticketScope = "group"
				_, err := testPool.Exec(ctx, `UPDATE organizations SET ticket_visibility='project_isolated' WHERE id=$1`, current)
				require.NoError(t, err)
			}
			_, err := testPool.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1`, env.mem2)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,ticket_scope) VALUES($1,$2,$3)`, env.mem2, current, ticketScope)
			require.NoError(t, err)
			oldTicket, newTicket := identityTicket(t, old, env.mem1), identityTicket(t, current, env.mem1)
			gate := ticketservice.NewResource(repository.NewTicketRepo(testPool), ticketservice.NewPgxScopeResolver(testPool), env.deleg)
			assert.Zero(t, identityFilterCount(t, gate, env.mem2, oldTicket), "same path is only a candidate; org_id ancestry must agree")
			actions := []string{"read"}
			if kind == "group" {
				actions = append(actions, "assign")
			}
			for _, action := range actions {
				ok, err := gate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.mem2, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(oldTicket), Action: action})
				require.NoError(t, err)
				assert.False(t, ok, action)
			}
			require.Equal(t, 1, identityFilterCount(t, gate, env.mem2, newTicket))
			ok, err := gate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.mem2, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(newTicket), Action: "read"})
			require.NoError(t, err)
			require.True(t, ok)
			if kind == "group" {
				ok, err = gate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.mem2, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(newTicket), Action: "assign"})
				require.NoError(t, err)
				require.True(t, ok)
			}
		})
	}
}

func TestIAMGuard_IndependentRootSourceKeepsPausedAndHistoricalTargets(t *testing.T) {
	for _, kind := range []string{"paused", "historical"} {
		t.Run(kind, func(t *testing.T) {
			resetRBACTables(t)
			env := setupDelegation(t)
			ctx := context.Background()
			_, err := testPool.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1`, env.mem2)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,ticket_scope) VALUES($1,1,'group')`, env.mem2)
			require.NoError(t, err)
			if kind == "paused" {
				_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, env.pID)
			} else {
				_, err = testPool.Exec(ctx, `UPDATE organizations SET deleted_at=NOW() WHERE id=$1`, env.vgID)
			}
			require.NoError(t, err)
			ticketID := identityTicket(t, env.vgID, env.mem1)
			gate := ticketservice.NewResource(repository.NewTicketRepo(testPool), ticketservice.NewPgxScopeResolver(testPool), env.deleg)
			require.Equal(t, 1, identityFilterCount(t, gate, env.mem2, ticketID), "independent root source remains qualified")
			for _, action := range []string{"read", "assign"} {
				ok, err := gate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.mem2, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(ticketID), Action: action})
				require.NoError(t, err)
				require.True(t, ok, action)
			}
		})
	}
}

func TestIAMGuard_ArchiveOwnerUsesIdentityAfterRecreatedOrganizationMoves(t *testing.T) {
	resetRBACTables(t)
	env := setupDelegation(t)
	ctx := trustedIAMTestContext()
	ticketID := identityTicket(t, env.vgID, env.mem1)
	_, err := testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=ARRAY[$1::bigint] WHERE id=$2`, env.owner, env.pID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE organizations SET deleted_at=NOW() WHERE id=ANY($1)`, []int64{env.pID, env.vgID})
	require.NoError(t, err)
	current := identityRecreate(t, env.pID, false)
	destination := iambOrg(t, "identity_move_"+uniqueSuffix())
	require.NoError(t, grantOrgService(t).Move(ctx, &model.MoveOrgRequest{ID: current, NewParentID: &destination}))
	gate := ticketservice.NewResource(repository.NewTicketRepo(testPool), ticketservice.NewPgxScopeResolver(testPool), env.deleg)
	require.Equal(t, 1, identityFilterCount(t, gate, env.owner, ticketID), "BK20 list uses the original actual org identity")
	ok, err := gate.Authorize(ctx, resource.AuthorizeRequest{UserID: env.owner, Roles: []string{"viewer"}, ResourceID: fmt.Sprint(ticketID), Action: "read"})
	require.NoError(t, err)
	require.True(t, ok, "a moved redundant path must not contradict proven historical parent IDs")
}

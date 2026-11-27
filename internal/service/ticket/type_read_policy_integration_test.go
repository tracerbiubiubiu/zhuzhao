//go:build integration

package ticket

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestNewTicketTypeRestrictsOrdinaryAllScope(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()
	code := "restricted_" + uniqueSuffix()
	_, err := env.svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	publishD01(t, env.svc, code, env.u1)
	tk, err := env.svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, OrgID: env.d1}, env.u1)
	require.NoError(t, err)
	ordinary := createB2User(t, "reader_"+uniqueSuffix(), "READ"+uniqueSuffix())
	bindOrgMemberScope(t, ordinary, env.d1, ScopeAll, nil)
	env.roles[ordinary] = []string{"operator"}
	_, err = env.svc.Get(ctx, tk.ID, ordinary)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	list, err := env.svc.List(ctx, model.TicketListQuery{TypeCode: code}, ordinary)
	require.NoError(t, err)
	require.Zero(t, list.Total)
	_, err = env.svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{ordinary}}, ordinary)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	_, err = env.svc.Get(ctx, tk.ID, env.u1)
	require.NoError(t, err)
}

func TestRoleRestrictedTicketFollowsCurrentOrgRole(t *testing.T) {
	env := setupB2(t)
	ctx := context.Background()
	code := "role_" + uniqueSuffix()
	var parent, child int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority) VALUES($1::text,$1::text,20) RETURNING id`, code).Scan(&parent))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,parent_id,priority) VALUES($1::text,$1::text,$2,30) RETURNING id`, code+"child", parent).Scan(&child))
	_, err := testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2)`, env.d2, child)
	require.NoError(t, err)
	_, err = env.svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code, ReadPolicy: "role_restricted", ReadRoleCodes: []string{code}})
	require.NoError(t, err)
	publishD01(t, env.svc, code, env.u1)
	tk, err := env.svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, OrgID: env.d1, Title: code}, env.u1)
	require.NoError(t, err)
	_, err = env.svc.Get(ctx, tk.ID, env.u2)
	require.NoError(t, err)
	_, err = env.svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{env.u2}}, env.u2)
	requireErrCode(t, err, errcode.ErrNoPermission.Code)
	_, err = testPool.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, parent)
	require.NoError(t, err)
	_, err = env.svc.Get(ctx, tk.ID, env.u2)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	_, err = env.svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code + "invalid", Name: code, ReadPolicy: "role_restricted", ReadRoleCodes: []string{code}})
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	// A same-organization colleague gets no implicit read access. A deliberate
	// CC grant is live, independent of the disabled role.
	env.roles[env.u1] = []string{"admin"}
	_, err = env.svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{env.colleague}}, env.u1)
	require.NoError(t, err)
	_, err = env.svc.Get(ctx, tk.ID, env.colleague)
	require.NoError(t, err)
	require.NoError(t, env.svc.RemoveCC(ctx, &model.RemoveTicketCCRequest{TicketID: tk.ID, UserID: env.colleague}, env.colleague))
	_, err = env.svc.Get(ctx, tk.ID, env.colleague)
	requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	requireErrCode(t, repository.NewRoleRepo(testPool).Delete(ctx, parent, code), errcode.ErrRoleInUse.Code)
}

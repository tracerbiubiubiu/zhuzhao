//go:build integration

package service_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"
	"github.com/gin-gonic/gin"
	pgxadapter "github.com/noho-digital/casbin-pgx-adapter"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type iamFailReloadAdapter struct {
	persist.Adapter
	fail atomic.Bool
}

func (a *iamFailReloadAdapter) LoadPolicy(m casbinmodel.Model) error {
	if a.fail.Load() {
		return errors.New("synthetic policy reload unavailable")
	}
	return a.Adapter.LoadPolicy(m)
}

func TestIAMGuard_FailedPolicyProjectionClosesCachedL1UntilRestart(t *testing.T) {
	resetRBACTables(t)
	ctx := context.Background()
	super := rbacTestRole(t, "superadmin", 1, true)
	role := rbacTestRole(t, "projection_reader", 60, false)
	actor := rbacTestUser(t, "projection_operator", "PROJECTION_OPERATOR", []int64{super.ID})
	reader := rbacTestUser(t, "projection_reader", "PROJECTION_READER", []int64{role.ID})
	_, err := testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::projection_reader','/api/v1/users','GET'),('p','role::projection_reader','/api/v1/assignment-rules','POST')`)
	require.NoError(t, err)
	base, err := pgxadapter.NewAdapterWithPool(testPool)
	require.NoError(t, err)
	adapter := &iamFailReloadAdapter{Adapter: base}
	m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m, adapter)
	require.NoError(t, err)
	users, roles, menus := repository.NewUserRepo(testPool), repository.NewRoleRepo(testPool), repository.NewMenuRepo(testPool)
	rbac := service.NewRBACService(roles, users, menus, enforcer)
	_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,1)`, reader)
	require.NoError(t, err)
	counted := &iamCountedPeople{repo: repository.NewTicketPeopleRepo(testPool)}
	ticketSvc := ticketservice.NewTicketService(testPool, repository.NewTicketRepo(testPool), repository.NewOrgRepo(testPool), resource.NewRegistry(), rbac, service.NewOrgDelegationService(testPool), nil, nil, nil)
	people := ticketservice.NewPeopleService(ticketSvc, enforcer, counted)
	query := model.TicketPeopleQuery{Purpose: "rule-target", OrgID: 1, Query: "projection"}
	candidates, err := people.Search(ctx, query, reader)
	require.NoError(t, err)
	require.NotEmpty(t, candidates)
	counted.calls = 0

	request := func(e *casbin.SyncedEnforcer, self bool) int {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("userID", reader); c.Next() })
		if self {
			r.Use(middleware.SelfService())
		}
		r.Use(middleware.CasbinAuth(e, rbac, nil))
		if self {
			r.GET("/api/v1/user/profile", handler.NewUserHandler(service.NewUserService(testPool, users, roles, nil, nil, nil), nil).GetProfile)
		} else {
			r.GET("/api/v1/users", handler.NewUserHandler(service.NewUserService(testPool, users, roles, nil, nil, nil), nil).List)
		}
		path := "/api/v1/users"
		if self {
			path = "/api/v1/user/profile"
		}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
		return w.Code
	}
	require.Equal(t, http.StatusOK, request(enforcer, false))
	adapter.fail.Store(true)
	operationCtx, operationID, err := service.IAMOperationContext(service.TrustedIAMInternal(ctx), "repair-policy-only-"+uniqueSuffix())
	require.NoError(t, err)
	err = rbac.AssignMenus(operationCtx, &model.AssignMenusRequest{RoleID: role.ID}, actor)
	require.ErrorIs(t, err, errcode.ErrPolicyReloadFailed)
	var policies int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM casbin_rule WHERE ptype='p' AND v0='role::projection_reader'`).Scan(&policies))
	require.Zero(t, policies, "DB revocation committed despite reload error")
	var repaired bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT f.policy_changed AND cardinality(f.added_ids)=0 AND cardinality(f.removed_ids)=0 FROM iam_command_receipts r JOIN iam_audit_facts f ON f.record_id=r.record_id WHERE r.actor_id=$1 AND r.operation_id=$2`, actor, operationID).Scan(&repaired))
	require.True(t, repaired)
	require.ErrorIs(t, rbac.AssignMenus(operationCtx, &model.AssignMenusRequest{RoleID: role.ID}, actor), errcode.ErrPolicyReloadFailed)

	allowed, err := enforcer.Enforce("role::projection_reader", "/api/v1/users", "GET")
	require.NoError(t, err)
	require.True(t, allowed, "the failed adapter deliberately leaves old memory intact")
	require.Equal(t, http.StatusServiceUnavailable, request(enforcer, false), "all cache-dependent subsequent L1 requests must close")
	candidates, err = people.Search(ctx, query, reader)
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable, "self-service people static action cannot consume an unavailable old cache")
	require.Nil(t, candidates)
	require.Zero(t, counted.calls, "rejected request must not invoke SearchEnabled")
	ccTicket := identityTicket(t, 1, reader)
	candidates, err = people.Search(ctx, model.TicketPeopleQuery{Purpose: "cc", TicketID: ccTicket, Query: "projection"}, reader)
	require.NoError(t, err)
	require.NotEmpty(t, candidates)
	require.Equal(t, 1, counted.calls, "cc keeps its original DB L3 without static p")

	require.Equal(t, http.StatusOK, request(enforcer, true), "self-service reads do not consume cached p")
	// Controlled restart builds a new enforcer only after loading current DB p.
	adapter.fail.Store(false)
	restarted, err := casbin.NewSyncedEnforcer(m.Copy(), adapter)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, request(restarted, false))
}

type iamCountedPeople struct {
	repo  *repository.TicketPeopleRepo
	calls int
}

func (p *iamCountedPeople) SearchEnabled(ctx context.Context, q string, orgID int64) ([]model.TicketPerson, error) {
	p.calls++
	return p.repo.SearchEnabled(ctx, q, orgID)
}

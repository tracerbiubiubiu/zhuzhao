package ticket

import (
	"context"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// RiskWorkflowRoute is a required current L1 policy check chosen by the trusted
// workflow adapter. It grants no resource action and does not mint HTTP purpose.
type RiskWorkflowRoute struct{ Path, Method string }

type riskWorkflowAuthorizationKey struct{}
type riskWorkflowAuthorization struct {
	service *Service
	tx      pgx.Tx
	actor   int64
}

// WithRiskWorkflowAuthorizationTx refreshes the actual admitted identity and
// roles on the exact owning C04 transaction. The callback retains the original
// middleware purpose; it receives only a service-private current role snapshot.
// It must propagate errors to its transaction owner and must not commit.
func (s *Service) WithRiskWorkflowAuthorizationTx(ctx context.Context, tx pgx.Tx, actor int64, routes []RiskWorkflowRoute, run func(context.Context) error) error {
	if s == nil || s.ticketRepo == nil || run == nil || actor <= 0 || len(routes) == 0 {
		return errcode.ErrInvalidParams
	}
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return err
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actor || purpose.SelfService {
		return errcode.ErrNoPermission
	}
	required := make([]ticketPolicyRoute, 0, len(routes))
	includesPurpose := false
	for _, route := range routes {
		if !strings.HasPrefix(route.Path, "/api/v1/") || strings.ContainsAny(route.Path, " \t\r\n") || (route.Method != "GET" && route.Method != "POST") {
			return errcode.ErrInvalidParams
		}
		required = append(required, ticketPolicyRoute{path: route.Path, method: route.Method})
		includesPurpose = includesPurpose || (route.Path == purpose.Path && route.Method == purpose.Method)
	}
	if !includesPurpose {
		return errcode.ErrNoPermission
	}
	if err := s.freshCommandCredential(ctx, actor, purpose); err != nil {
		return err
	}
	roles, err := s.freshCommandRoles(ctx, actor, purpose.Model, required...)
	if err != nil {
		return err
	}
	current := context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
	current = context.WithValue(current, riskWorkflowAuthorizationKey{}, riskWorkflowAuthorization{service: s, tx: tx, actor: actor})
	return run(current)
}

// RiskWorkflowRouteGrantsTx projects current L1 permissions onto an authorized
// workflow read. Each write still performs its own route and object checks.
func (s *Service) RiskWorkflowRouteGrantsTx(ctx context.Context, tx pgx.Tx, actor int64, routes []RiskWorkflowRoute) ([]bool, error) {
	proof, ok := ctx.Value(riskWorkflowAuthorizationKey{}).(riskWorkflowAuthorization)
	if !ok || proof.service != s || proof.tx != tx || proof.actor != actor {
		return nil, errcode.ErrNoPermission
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actor || purpose.SelfService {
		return nil, errcode.ErrNoPermission
	}
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, err
	}
	roles, policies, err := repository.NewUserRepo(s.db).CurrentIAMAuthorization(ctx, actor)
	if err != nil {
		return nil, err
	}
	required := make([]ticketPolicyRoute, len(routes))
	for i, route := range routes {
		required[i] = ticketPolicyRoute{path: route.Path, method: route.Method}
	}
	return commandRouteGrants(purpose.Model, roles, policies, required)
}

// CheckRiskWorkflowReadTx retains both current ticket GET L1 and the original
// ticket resource read rules. Risk/receipt visibility never grants ticket read.
// Only the callback context above, for this exact service/tx/actor, qualifies.
func (s *Service) CheckRiskWorkflowReadTx(ctx context.Context, tx pgx.Tx, actor, ticketID int64) error {
	if s == nil || s.ticketRepo == nil || actor <= 0 || ticketID <= 0 {
		return errcode.ErrInvalidParams
	}
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return err
	}
	proof, ok := ctx.Value(riskWorkflowAuthorizationKey{}).(riskWorkflowAuthorization)
	if !ok || proof.service != s || proof.tx != tx || proof.actor != actor {
		return errcode.ErrNoPermission
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actor || purpose.SelfService {
		return errcode.ErrNoPermission
	}
	if err := s.freshCommandCredential(ctx, actor, purpose); err != nil {
		return err
	}
	roles, err := s.freshCommandRoles(ctx, actor, purpose.Model, ticketPolicyRoute{path: "/api/v1/tickets/" + strconv.FormatInt(ticketID, 10), method: "GET"})
	if err != nil {
		return err
	}
	current := context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
	return s.authorizeCheck(current, actor, "read", strconv.FormatInt(ticketID, 10))
}

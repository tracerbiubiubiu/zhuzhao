package ticket

import (
	"context"
	"strconv"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func (s *Service) CanReadRiskDispatchTicket(ctx context.Context, actorID, ticketID int64) bool {
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actorID {
		return false
	}
	if err := s.freshCommandCredential(ctx, actorID, purpose); err != nil {
		return false
	}
	path := "/api/v1/tickets/" + strconv.FormatInt(ticketID, 10)
	roles, err := s.freshCommandRoles(ctx, actorID, purpose.Model, ticketPolicyRoute{path: path, method: "GET"})
	if err != nil {
		return false
	}
	ctx = context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actorID, roles: roles})
	_, err = s.Get(ctx, ticketID, actorID)
	return err == nil
}

// RiskDispatchVisibleTicketsWhere resolves the current ticket list scope for
// risk dispatch statistics. Risk membership confers no ticket permissions.
func (s *Service) RiskDispatchVisibleTicketsWhere(ctx context.Context, actorID int64) (string, []any, bool, error) {
	const statsPath = "/api/v1/risk-items/dispatch-statistics"
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actorID || purpose.Path != statsPath || purpose.Method != "GET" || purpose.SelfService {
		return "", nil, false, errcode.ErrNoPermission
	}
	if err := s.freshCommandCredential(ctx, actorID, purpose); err != nil {
		return "", nil, false, err
	}
	roles, err := s.freshCommandRoles(ctx, actorID, purpose.Model,
		ticketPolicyRoute{path: statsPath, method: "GET"},
		ticketPolicyRoute{path: "/api/v1/tickets", method: "GET"})
	if err != nil {
		return "", nil, false, err
	}
	ctx = context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actorID, roles: roles})
	scope, err := s.resolveListFilter(ctx, actorID)
	return scope.Where, scope.Args, scope.Unscoped, err
}

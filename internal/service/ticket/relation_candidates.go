package ticket

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	ticketListPath     = "/api/v1/tickets"
	ticketRelationPath = "/api/v1/tickets/relations"
)

// RelationCandidates previews the existing dual-update authorization. The write
// still checks both ticket rows again under its own transaction.
func (s *Service) RelationCandidates(ctx context.Context, sourceID, actor int64, keyword string) ([]model.TicketRelationCandidate, error) {
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || actor <= 0 || purpose.UserID != actor || purpose.Path != ticketListPath || purpose.Method != "GET" || purpose.SelfService {
		return nil, errcode.ErrNoPermission
	}
	keyword = strings.TrimSpace(keyword)
	if sourceID <= 0 || keyword == "" || utf8.RuneCountInString(keyword) > 50 {
		return nil, errcode.ErrInvalidParams
	}
	var candidates []model.TicketRelationCandidate
	err := repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, _ pgx.Tx) error {
		roles, err := s.freshCommandRoles(ctx, actor, purpose.Model,
			ticketPolicyRoute{path: ticketListPath, method: "GET"},
			ticketPolicyRoute{path: ticketRelationPath, method: "POST"},
		)
		if err != nil {
			return err
		}
		ctx = context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
		if err := s.authorizeCheck(ctx, actor, "update", strconv.FormatInt(sourceID, 10)); err != nil {
			return err
		}
		// A global administrator's resource bypass doesn't establish existence.
		if _, err := s.ticketRepo.GetByID(ctx, sourceID); err != nil {
			return err
		}
		filter := resource.Filter{Where: "tickets.id <> $1", Args: []any{sourceID}}
		if !HasRole(roles, "admin") && !HasRole(roles, "superadmin") {
			// Both branches grant visibility and update (Resource.canOperate).
			// Assignee, read scope and CC alone deliberately do not enter this filter.
			filter.Where = "tickets.id <> $2 AND (tickets.created_by = $1 OR " + delegatedVisibilitySQL + ")"
			filter.Args = []any{actor, sourceID}
		}
		candidates, err = s.ticketRepo.SearchRelationCandidates(ctx, filter, keyword)
		return err
	})
	return candidates, err
}

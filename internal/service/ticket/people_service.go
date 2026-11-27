package ticket

import (
	"context"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/casbin/casbin/v3"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// PeopleSearcher is intentionally narrower than the user administration repository.
type PeopleSearcher interface {
	SearchEnabled(context.Context, string, int64) ([]model.TicketPerson, error)
}

// PeopleService checks the original action before exposing candidate identities.
type PeopleService struct {
	ticket   *Service
	enforcer *casbin.SyncedEnforcer
	repo     PeopleSearcher
}

// NewPeopleService creates the ticket-context people search service.
func NewPeopleService(ticket *Service, enforcer *casbin.SyncedEnforcer, repo PeopleSearcher) *PeopleService {
	return &PeopleService{ticket: ticket, enforcer: enforcer, repo: repo}
}

// Search returns no identities until the concrete write context is authorized.
func (s *PeopleService) Search(ctx context.Context, q model.TicketPeopleQuery, actor int64) ([]model.TicketPerson, error) {
	q.Query = strings.TrimSpace(q.Query)
	if actor <= 0 || utf8.RuneCountInString(q.Query) < 2 || utf8.RuneCountInString(q.Query) > 50 {
		return nil, errcode.New(errcode.ErrInvalidParams.Code, "请输入2–50个字符搜索人员")
	}
	var path, action string
	orgID := int64(0)
	switch q.Purpose {
	case "transfer":
		if q.TicketID <= 0 || q.OrgID != 0 || q.TypeCode != "" {
			return nil, errcode.ErrInvalidParams
		}
		path = "/api/v1/tickets/transfer"
	case "assign", "cc":
		if q.TicketID <= 0 || q.OrgID != 0 || q.TypeCode != "" {
			return nil, errcode.ErrInvalidParams
		}
		action = "assign"
		path = "/api/v1/tickets/assign"
		if q.Purpose == "cc" {
			action = "update"
			path = ""
		} // AddCC's original route is dynamic L2/L3, without a static L1 code.
	case "create-supervisor":
		if q.TicketID != 0 || q.OrgID <= 0 || q.TypeCode == "" {
			return nil, errcode.ErrInvalidParams
		}
		path = "/api/v1/tickets"
	case "rule-target":
		if q.TicketID != 0 || q.OrgID <= 0 || q.TypeCode != "" {
			return nil, errcode.ErrInvalidParams
		}
		path = "/api/v1/assignment-rules"
		orgID = q.OrgID
	default:
		return nil, errcode.ErrInvalidParams
	}
	roles, err := s.ticket.getRoles(ctx, actor)
	if err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return nil, errcode.ErrNoRoles
	}
	if path != "" {
		if !middleware.PolicyProjectionAvailable(s.enforcer) {
			return nil, errcode.ErrServiceUnavailable
		}
		allowed := false
		failed := false
		for _, role := range roles {
			ok, e := s.enforcer.Enforce("role::"+role, path, "POST")
			if e != nil {
				failed = true
			}
			if e == nil && ok {
				allowed = true
				break
			}
		}
		if !middleware.PolicyProjectionAvailable(s.enforcer) {
			return nil, errcode.ErrServiceUnavailable
		}

		if !allowed {
			if failed {
				return nil, errcode.ErrServiceUnavailable
			}
			return nil, errcode.ErrNoPermission
		}
	}
	if q.Purpose == "transfer" {
		return s.ticket.transferCandidates(ctx, q, actor)
	}
	if action != "" {
		if err := s.ticket.authorizeCheck(ctx, actor, action, strconv.FormatInt(q.TicketID, 10)); err != nil {
			return nil, err
		}
		// Admin resource bypass still must not allow a nonexistent/closed assignment context.
		tk, err := s.ticket.ticketRepo.GetByID(ctx, q.TicketID)
		if err != nil {
			return nil, err
		}
		if q.Purpose == "assign" && (tk.Status == StatusClosed || tk.Status == StatusCancelled) {
			return nil, errcode.ErrTicketAlreadyClosed
		}
	}
	if q.Purpose == "create-supervisor" {
		// Reuse the current Create port: same-branch membership or its exact global org:member exemption.
		member, err := s.ticket.orgRepo.IsInOrgBranch(ctx, q.OrgID, actor)
		if err != nil {
			return nil, err
		}
		if !member && !s.ticket.isGlobalOrgAdmin(ctx, actor) {
			return nil, errcode.ErrNoPermission
		}
		if _, err := s.ticket.orgRepo.FindByID(ctx, q.OrgID); err != nil {
			return nil, err
		}
		typeRow, err := s.ticket.ticketRepo.GetTicketType(ctx, q.TypeCode)
		if err != nil {
			return nil, err
		}
		if !typeRow.IsActive {
			return nil, errcode.ErrTicketTypeNotFound
		}
		if err := validateTypeGraph(typeRow.States, typeRow.Transitions); err != nil {
			return nil, err
		}
	}
	return s.repo.SearchEnabled(ctx, q.Query, orgID)
}

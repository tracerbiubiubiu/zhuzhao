package riskworkflow

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

const (
	acceptRequestPath  = "/api/v1/risk-items/remediation-accept-request"
	acceptDecisionPath = "/api/v1/risk-items/remediation-accept-decision"
)

// Acceptance commands are an in-process, linked-ticket slice. Routing and menu
// assembly belong to the app owner. The existing ticket publication is always
// authoritative; these commands neither publish nor synthesize a graph.
type AcceptanceRequestCommand struct {
	riskremediation.AcceptanceRequest
	TicketInput ticket.RiskWorkflowTicketInput
}
type AcceptanceDecisionCommand struct {
	riskremediation.AcceptanceDecision
	TicketInput ticket.RiskWorkflowTicketInput
}

func (s *Service) RequestAcceptance(ctx context.Context, c AcceptanceRequestCommand) (riskremediation.Result, error) {
	if c.TicketBinding != "" || c.TicketID <= 0 || c.ExpectedTicketRevision <= 0 {
		return riskremediation.Result{}, errcode.ErrInvalidParams
	}
	input, binding, err := ticket.PrepareRiskWorkflowTicketInput(c.TicketInput)
	if err != nil {
		return riskremediation.Result{}, err
	}
	c.TicketBinding = binding
	return s.acceptanceCommand(ctx, c.ActorID, acceptRequestPath, input, func(ctx context.Context, tx pgx.Tx, callbacks riskremediation.Callbacks) (riskremediation.Result, error) {
		return riskremediation.New(s.pool, callbacks).RequestAcceptanceTx(ctx, tx, c.AcceptanceRequest)
	})
}
func (s *Service) DecideAcceptance(ctx context.Context, c AcceptanceDecisionCommand) (riskremediation.Result, error) {
	if c.TicketBinding != "" || c.TicketID <= 0 || c.ExpectedTicketRevision <= 0 {
		return riskremediation.Result{}, errcode.ErrInvalidParams
	}
	input, binding, err := ticket.PrepareRiskWorkflowTicketInput(c.TicketInput)
	if err != nil {
		return riskremediation.Result{}, err
	}
	c.TicketBinding = binding
	return s.acceptanceCommand(ctx, c.ActorID, acceptDecisionPath, input, func(ctx context.Context, tx pgx.Tx, callbacks riskremediation.Callbacks) (riskremediation.Result, error) {
		// The kernel resolves AcceptanceID before its Prelock. Check current
		// ticket read first so a caller lacking that right cannot distinguish
		// existing acceptance IDs from absent ones. This takes no domain locks,
		// issues no kernel grant, and does not constrain historical replay to
		// live links. The kernel still repeats authorization on its own child.
		err := s.tickets.WithRiskWorkflowAuthorizationTx(ctx, tx, c.ActorID, []ticket.RiskWorkflowRoute{{Path: acceptDecisionPath, Method: "POST"}}, func(current context.Context) error {
			return s.tickets.CheckRiskWorkflowReadTx(current, tx, c.ActorID, c.TicketID)
		})
		if err != nil {
			return riskremediation.Result{}, err
		}
		return riskremediation.New(s.pool, callbacks).DecideAcceptanceTx(ctx, tx, c.AcceptanceDecision)
	})
}

func (s *Service) acceptanceCommand(ctx context.Context, actor int64, path string, input ticket.RiskWorkflowTicketInput, run func(context.Context, pgx.Tx, riskremediation.Callbacks) (riskremediation.Result, error)) (riskremediation.Result, error) {
	if s == nil || s.pool == nil || s.tickets == nil {
		return riskremediation.Result{}, errcode.ErrServiceUnavailable
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actor || purpose.Path != path || purpose.Method != "POST" || purpose.SelfService {
		return riskremediation.Result{}, errcode.ErrNoPermission
	}
	var result riskremediation.Result
	err := repository.WithIAMCommandGuard(ctx, s.pool, func(ctx context.Context, outer pgx.Tx) error {
		var err error
		result, err = run(ctx, outer, s.acceptanceCallbacks(actor, path, input))
		return err
	})
	if err != nil {
		return riskremediation.Result{}, err
	}
	return result, nil
}

// Like writeCallbacks, admission is rebuilt on the kernel's exact IAM child;
// type/org locks precede group/risk/ticket/link locks. Acceptance differs in its
// action qualification and ticket effects, so fixed/review callbacks stay closed.
func (s *Service) acceptanceCallbacks(actor int64, path string, input ticket.RiskWorkflowTicketInput) riskremediation.Callbacks {
	var current context.Context
	var child pgx.Tx
	var scope *ticket.RiskWorkflowScope
	orgs := map[int64]bool{}
	exact := func(ctx context.Context, tx pgx.Tx) error {
		if current == nil || child != tx || !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, s.pool) != tx {
			return errcode.ErrNoPermission
		}
		return nil
	}
	read := func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
		if err := exact(ctx, tx); err != nil {
			return err
		}
		if a.ActorID != actor || a.TicketID <= 0 {
			return errcode.ErrNoPermission
		}
		if err := s.tickets.CheckRiskWorkflowReadTx(current, tx, actor, a.TicketID); err != nil {
			return err
		}
		if a.RiskID > 0 {
			_, _, _, err := repository.GetRiskItemVisible(current, tx, a.RiskID, actor)
			if errors.Is(err, repository.ErrRiskNotFound) {
				return errcode.ErrNotFound
			}
			if err != nil {
				return errcode.ErrServiceUnavailable
			}
		}
		return nil
	}
	return riskremediation.Callbacks{
		Admit: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
			if a.ActorID != actor || !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, s.pool) != tx {
				return errcode.ErrNoPermission
			}
			return s.tickets.WithRiskWorkflowAuthorizationTx(ctx, tx, actor, []ticket.RiskWorkflowRoute{{Path: path, Method: "POST"}}, func(fresh context.Context) error { current = fresh; child = tx; return nil })
		},
		Prelock: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
			if err := read(ctx, tx, a); err != nil {
				return err
			}
			if a.Action == "receipt" {
				return nil // replay reads the original scope without consuming an edge
			}
			if a.Action != "accept_request" && a.Action != "accept_confirm" {
				return errcode.ErrServiceUnavailable
			}
			rows, err := tx.Query(current, `SELECT DISTINCT r.org_id FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id JOIN risk_dispatch_groups g ON g.id=l.group_id WHERE g.ticket_id=$1 AND l.ended_at IS NULL ORDER BY r.org_id`, a.TicketID)
			if err != nil {
				return errcode.ErrServiceUnavailable
			}
			ids, err := pgx.CollectRows(rows, pgx.RowTo[int64])
			if err != nil {
				return errcode.ErrServiceUnavailable
			}
			for _, id := range ids {
				orgs[id] = true
			}
			scope, err = s.tickets.PrelockRiskWorkflowTx(current, tx, a.TicketID, actor, ids...)
			if err != nil {
				return err
			}
			if scope.DefinitionVersion() != input.DefinitionVersion {
				return errcode.ErrConflict
			}
			if scope.Status() != "in_progress" && scope.Status() != "pending_verify" {
				return errcode.ErrConflict
			}
			return nil
		},
		Authorize: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
			if err := read(ctx, tx, a); err != nil {
				return err
			}
			if a.Action == "receipt" {
				return nil
			}
			if a.RiskID <= 0 || !orgs[a.OrgID] {
				return errcode.ErrConflict
			}
			switch a.Action {
			case "accept_request":
				if actor != a.OwnerID {
					return errcode.ErrNoPermission
				}
				return nil
			case "accept_confirm":
				manage, err := repository.CanManageRiskItem(current, tx, a.RiskID, actor)
				if err != nil {
					return errcode.ErrServiceUnavailable
				}
				if !manage {
					return errcode.ErrNoPermission
				}
				return nil // kernel additionally rejects the original applicant
			default:
				return errcode.ErrServiceUnavailable
			}
		},
		TicketPolicy: func(ctx context.Context, tx pgx.Tx, c riskremediation.TicketCheck) (riskremediation.Policy, error) {
			if err := exact(ctx, tx); err != nil {
				return riskremediation.Policy{}, err
			}
			if c.ActorID != actor || (c.Action != "accept_request" && c.Action != "accept_confirm") || scope == nil || scope.Revision() != c.ExpectedRevision {
				return riskremediation.Policy{}, errcode.ErrConflict
			}
			if c.Action == "accept_request" {
				if scope.Status() != "pending_verify" {
					return riskremediation.Policy{}, errcode.ErrConflict
				}
				// A pending application blocks normal completion. Require both
				// published decision exits before creating or renewing it.
				if !scope.Allows("closed") || !scope.Allows("in_progress") {
					return riskremediation.Policy{}, errcode.ErrTicketInvalidTransition
				}
			}
			return riskremediation.Policy{ReviewGate: true}, nil
		},
		Evidence: func(context.Context, pgx.Tx, riskremediation.EvidenceCheck) error {
			return errcode.ErrServiceUnavailable
		},
		ApplyTicket: func(ctx context.Context, tx pgx.Tx, e riskremediation.TicketEffect) (int64, error) {
			if err := exact(ctx, tx); err != nil {
				return 0, err
			}
			if scope == nil || e.ActorID != actor || scope.Revision() != e.ExpectedRevision {
				return 0, errcode.ErrConflict
			}
			to := ""
			switch e.Action {
			case "accept_request":
				if e.TerminalAllowed {
					return 0, errcode.ErrConflict
				}
			case "accept_confirm":
				if e.TerminalAllowed {
					to = "closed"
				}
				for _, item := range e.Items {
					if item.Result == "rejected" {
						// Older candidates could leave pending applications on
						// tickets already in progress. Reject without a self edge.
						to = ""
						if scope.Status() != "in_progress" {
							to = "in_progress"
						}
					}
				}
			default:
				return 0, errcode.ErrServiceUnavailable
			}
			if to == "" {
				// A decision without an outgoing edge only records risk facts. Do not
				// invent a self edge, move a ticket to review, or swallow a patch.
				if string(input.CustomDataPatch) != "{}" || input.Comment != "" {
					return 0, errcode.ErrServiceUnavailable
				}
				return scope.Revision(), nil
			}
			if !scope.Allows(to) {
				return 0, errcode.ErrTicketInvalidTransition
			}
			req := &model.TransitionTicketRequest{ID: e.TicketID, To: to, ExpectedRevision: decimal(e.ExpectedRevision), DefinitionVersion: decimal(input.DefinitionVersion), CustomDataPatch: input.CustomDataPatch, Comment: input.Comment,
				IdempotencyKey: "risk-accept-" + strconv.FormatInt(actor, 10) + "-" + strconv.FormatInt(e.TicketID, 10) + "-" + strconv.FormatInt(e.ExpectedRevision, 10)}
			tk, err := s.tickets.ApplyRiskWorkflowTx(current, tx, scope, req)
			if err != nil {
				return 0, err
			}
			return tk.Revision, nil
		},
	}
}

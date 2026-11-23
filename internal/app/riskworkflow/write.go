package riskworkflow

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	commentref "github.com/tracerbiubiubiu/zhuzhao/internal/model/comment"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

const (
	finishPath = "/api/v1/risk-items/remediation-finish"
	reviewPath = "/api/v1/risk-items/remediation-review"
)

// Service is an in-process gate-on fixed/approve/reject workflow. App/router wiring is
// deliberately separate. It only accepts confirmed, already-bound attachments;
// acceptance, new uploads and alternative rule profiles stay closed.
// All three collaborators MUST own the same fact pool.
type Service struct {
	pool     *pgxpool.Pool
	tickets  *ticket.Service
	evidence *storageservice.BoundTicketEvidenceValidator
}

func NewService(pool *pgxpool.Pool, tickets *ticket.Service, evidence *storageservice.BoundTicketEvidenceValidator) *Service {
	return &Service{pool: pool, tickets: tickets, evidence: evidence}
}

// TicketInput is checked against the actual pin and normalized by the ticket
// owner. Callers cannot supply the kernel's internal TicketBinding.
type FinishCommand struct {
	riskremediation.Command
	Items       []riskremediation.Item
	TicketInput ticket.RiskWorkflowTicketInput
}
type ReviewCommand struct {
	riskremediation.Command
	Items       []riskremediation.ReviewItem
	TicketInput ticket.RiskWorkflowTicketInput
}

func (s *Service) Finish(ctx context.Context, c FinishCommand) (riskremediation.Result, error) {
	if c.TicketBinding != "" {
		return riskremediation.Result{}, errcode.ErrInvalidParams
	}
	input, binding, err := ticket.PrepareRiskWorkflowTicketInput(c.TicketInput)
	if err != nil {
		return riskremediation.Result{}, err
	}
	c.TicketBinding = binding
	for _, item := range c.Items {
		if item.Resolution != "fixed" {
			return riskremediation.Result{}, errcode.ErrServiceUnavailable
		}
	}
	return s.command(ctx, c.ActorID, finishPath, input, func(ctx context.Context, tx pgx.Tx, callbacks riskremediation.Callbacks) (riskremediation.Result, error) {
		return riskremediation.New(s.pool, callbacks).FinishTx(ctx, tx, riskremediation.FinishCommand{Command: c.Command, Items: c.Items})
	})
}
func (s *Service) Review(ctx context.Context, c ReviewCommand) (riskremediation.Result, error) {
	if c.TicketBinding != "" {
		return riskremediation.Result{}, errcode.ErrInvalidParams
	}
	input, binding, err := ticket.PrepareRiskWorkflowTicketInput(c.TicketInput)
	if err != nil {
		return riskremediation.Result{}, err
	}
	c.TicketBinding = binding
	for _, item := range c.Items {
		if item.Decision != "approve" && item.Decision != "reject" {
			return riskremediation.Result{}, errcode.ErrServiceUnavailable
		}
	}
	return s.command(ctx, c.ActorID, reviewPath, input, func(ctx context.Context, tx pgx.Tx, callbacks riskremediation.Callbacks) (riskremediation.Result, error) {
		return riskremediation.New(s.pool, callbacks).ReviewTx(ctx, tx, riskremediation.ReviewCommand{Command: c.Command, Items: c.Items})
	})
}
func (s *Service) command(ctx context.Context, actor int64, path string, input ticket.RiskWorkflowTicketInput, run func(context.Context, pgx.Tx, riskremediation.Callbacks) (riskremediation.Result, error)) (riskremediation.Result, error) {
	if s == nil || s.pool == nil || s.tickets == nil || s.evidence == nil {
		return riskremediation.Result{}, errcode.ErrServiceUnavailable
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actor || purpose.Path != path || purpose.Method != "POST" || purpose.SelfService {
		return riskremediation.Result{}, errcode.ErrNoPermission
	}
	var result riskremediation.Result
	err := repository.WithIAMCommandGuard(ctx, s.pool, func(ctx context.Context, outer pgx.Tx) error {
		callbacks := s.writeCallbacks(actor, path, input)
		var err error
		result, err = run(ctx, outer, callbacks)
		return err
	})
	if err != nil {
		return riskremediation.Result{}, err
	}
	return result, nil
}

// The kernel owns its IAM savepoint and all group/risk/ticket/link locks. Admit
// rebuilds current authorization INSIDE that exact child. Prelock only takes
// predecessor type/org locks; later callbacks never acquire a predecessor lock.
func (s *Service) writeCallbacks(actor int64, path string, input ticket.RiskWorkflowTicketInput) riskremediation.Callbacks {
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
		if a.ActorID != actor {
			return errcode.ErrNoPermission
		}
		if a.TicketID > 0 {
			if err := s.tickets.CheckRiskWorkflowReadTx(current, tx, actor, a.TicketID); err != nil {
				return err
			}
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
		IndependentApprover: denyIndependentAcceptanceApprover,
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
				return nil
			} // replay needs original read, never a consumed edge
			var err error
			rows, e := tx.Query(current, `SELECT DISTINCT r.org_id FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id JOIN risk_dispatch_groups g ON g.id=l.group_id WHERE g.ticket_id=$1 AND l.ended_at IS NULL ORDER BY r.org_id`, a.TicketID)
			if e != nil {
				return errcode.ErrServiceUnavailable
			}
			ids, e := pgx.CollectRows(rows, pgx.RowTo[int64])
			if e != nil {
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
			if !scope.HasEdge("in_progress", "pending_verify") || !scope.HasEdge("pending_verify", "pending_verify") || !scope.HasEdge("pending_verify", "closed") || !scope.HasEdge("pending_verify", "in_progress") {
				return errcode.ErrServiceUnavailable
			}
			return nil
		},
		Authorize: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
			if err := read(ctx, tx, a); err != nil {
				return err
			}
			if a.Action != "receipt" && a.RiskID > 0 && !orgs[a.OrgID] {
				return errcode.ErrConflict
			}
			switch a.Action {
			case "receipt":
				return nil
			case "finish":
				var handler int64
				if err := tx.QueryRow(current, `SELECT COALESCE(assigned_to,0) FROM tickets WHERE id=$1`, a.TicketID).Scan(&handler); err != nil {
					return errcode.ErrServiceUnavailable
				}
				if actor != handler {
					return errcode.ErrNoPermission
				}
				return nil
			case "review":
				if actor != a.OwnerID {
					return errcode.ErrNoPermission
				}
				return nil
			default:
				return errcode.ErrServiceUnavailable
			}
		},
		TicketPolicy: func(ctx context.Context, tx pgx.Tx, c riskremediation.TicketCheck) (riskremediation.Policy, error) {
			if err := exact(ctx, tx); err != nil {
				return riskremediation.Policy{}, err
			}
			if scope == nil || scope.Revision() != c.ExpectedRevision || (c.Action == "finish" && scope.Status() != "in_progress") || (c.Action == "review" && scope.Status() != "pending_verify") {
				return riskremediation.Policy{}, errcode.ErrConflict
			}
			return riskremediation.Policy{ReviewGate: true}, nil
		},
		Evidence: func(ctx context.Context, tx pgx.Tx, c riskremediation.EvidenceCheck) error {
			if err := read(ctx, tx, c.Access); err != nil {
				return err
			}
			if (c.Mode != riskremediation.EvidenceValidate && c.Mode != riskremediation.EvidenceBind) || (c.Historical && c.Mode != riskremediation.EvidenceValidate) {
				return errcode.ErrInvalidParams
			}
			files := make([]storageservice.BoundTicketEvidenceRef, 0, len(c.Evidence))
			comments := make([]commentref.EvidenceRef, 0, len(c.Evidence))
			for _, ref := range c.Evidence {
				switch ref.Kind {
				case "file":
					files = append(files, storageservice.BoundTicketEvidenceRef{ObjectKey: ref.Ref, Version: ref.Version})
				case "reference":
					comments = append(comments, commentref.EvidenceRef{Ref: ref.Ref, Version: ref.Version})
				default:
					return errcode.ErrInvalidParams
				}
			}
			// Bind is revalidation only: eligibility requires an existing exact binding.
			if err := s.tickets.ValidateTicketCommentEvidenceTx(current, tx, c.TicketID, comments); err != nil {
				return err
			}
			return s.evidence.ValidateBoundTicketEvidenceTx(current, tx, c.TicketID, files)
		},
		ApplyTicket: func(ctx context.Context, tx pgx.Tx, e riskremediation.TicketEffect) (int64, error) {
			if err := exact(ctx, tx); err != nil {
				return 0, err
			}
			if e.Action == "remediation_finish" {
				// This fixed profile enters review once for the entire live group.
				// A missing/conflicted sibling must not be stranded in remediation
				// behind a pending_verify ticket. All kernel writes are provisional;
				// rejecting here rolls them back with the exact owning savepoint.
				var incomplete bool
				err := tx.QueryRow(current, `SELECT EXISTS(
				 SELECT 1 FROM risk_ticket_links l JOIN risk_dispatch_groups g ON g.id=l.group_id
				 WHERE g.ticket_id=$1 AND l.ended_at IS NULL
				 AND l.remediation_state NOT IN ('pending_verify','resolved'))`, e.TicketID).Scan(&incomplete)
				if err != nil {
					return 0, errcode.ErrServiceUnavailable
				}
				if incomplete {
					return 0, errcode.ErrConflict
				}
			}
			to := "pending_verify"
			if e.TerminalAllowed {
				to = "closed"
			}
			for _, item := range e.Items {
				if item.Result == "rejected" {
					to = "in_progress"
					break
				}
			}
			if scope == nil || !scope.Allows(to) {
				return 0, errcode.ErrTicketInvalidTransition
			}
			req := &model.TransitionTicketRequest{ID: e.TicketID, To: to, ExpectedRevision: decimal(e.ExpectedRevision), DefinitionVersion: decimal(input.DefinitionVersion), CustomDataPatch: input.CustomDataPatch, Comment: input.Comment, IdempotencyKey: "risk-" + strconv.FormatInt(actor, 10) + "-" + strconv.FormatInt(e.TicketID, 10) + "-" + strconv.FormatInt(e.ExpectedRevision, 10)}
			tk, err := s.tickets.ApplyRiskWorkflowTx(current, tx, scope, req)
			if err != nil {
				return 0, err
			}
			return tk.Revision, nil
		},
	}
}
func decimal(n int64) json.RawMessage { raw, _ := json.Marshal(strconv.FormatInt(n, 10)); return raw }

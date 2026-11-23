package riskworkflow

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskreminder"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

const (
	assignHandlerPath    = "/api/v1/risk-items/remediation-assign-handler"
	acceptAssignmentPath = "/api/v1/risk-items/remediation-accept-assignment"
)

// AssignmentCommand is an in-process command. ActorID comes from authenticated
// context; handlers must parse decimal concurrency tokens without float coercion.
type AssignmentCommand struct {
	ActorID                int64                          `json:"-"`
	OperationID            string                         `json:"operation_id"`
	TicketID               int64                          `json:"ticket_id"`
	ExpectedTicketRevision int64                          `json:"expected_ticket_revision"`
	TicketInput            ticket.RiskWorkflowTicketInput `json:"ticket_input"`
}
type AssignHandlerCommand struct {
	AssignmentCommand
	AssignedToUserID int64 `json:"assigned_to_user_id"`
}
type AssignmentResult = riskremediation.Result

type AssignmentDependencies struct {
	Clock func() time.Time
	// Recorders run on the exact owning transaction after ticket/link locks.
	RecordAssignment func(context.Context, pgx.Tx, int64, int64, int64, time.Time) (*riskreminder.Assignment, bool, error)
	RecordAcceptance func(context.Context, pgx.Tx, int64, int64, time.Time) (*riskreminder.Assignment, bool, error)
}
type AssignmentService struct {
	pool    *pgxpool.Pool
	tickets *ticket.Service
	deps    AssignmentDependencies
}

func NewAssignmentService(pool *pgxpool.Pool, tickets *ticket.Service, deps AssignmentDependencies) *AssignmentService {
	return &AssignmentService{pool: pool, tickets: tickets, deps: deps}
}
func (s *AssignmentService) AssignHandler(ctx context.Context, c AssignHandlerCommand) (AssignmentResult, error) {
	if c.AssignedToUserID <= 0 || len(c.TicketInput.CustomDataPatch) > 0 {
		return AssignmentResult{}, errcode.ErrInvalidParams
	}
	return s.assignmentCommand(ctx, c.AssignmentCommand, c.AssignedToUserID)
}
func (s *AssignmentService) AcceptAssignment(ctx context.Context, c AssignmentCommand) (AssignmentResult, error) {
	return s.assignmentCommand(ctx, c, 0)
}

func (s *AssignmentService) assignmentCommand(ctx context.Context, c AssignmentCommand, target int64) (AssignmentResult, error) {
	if s == nil || s.pool == nil || s.tickets == nil || s.deps.Clock == nil || s.deps.RecordAssignment == nil || s.deps.RecordAcceptance == nil {
		return AssignmentResult{}, errcode.ErrServiceUnavailable
	}
	if c.ActorID <= 0 || c.TicketID <= 0 || c.ExpectedTicketRevision <= 0 || strings.TrimSpace(c.OperationID) == "" || len(c.OperationID) > 128 {
		return AssignmentResult{}, errcode.ErrInvalidParams
	}
	input, _, err := ticket.PrepareRiskWorkflowTicketInput(c.TicketInput)
	if err != nil {
		return AssignmentResult{}, err
	}
	c.TicketInput = input
	path, command := acceptAssignmentPath, "remediation_accept_assignment"
	if target > 0 {
		path, command = assignHandlerPath, "remediation_assign_handler"
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != c.ActorID || purpose.Path != path || purpose.Method != "POST" || purpose.SelfService {
		return AssignmentResult{}, errcode.ErrNoPermission
	}
	var out AssignmentResult
	err = repository.WithIAMCommandGuard(ctx, s.pool, func(ctx context.Context, outer pgx.Tx) error {
		// Own the child even when an enclosing guarded caller handles our error.
		// Failed SQL, facts and after-commit projections cannot escape that child.
		return repository.WithIAMSavepoint(ctx, s.pool, outer, func(ctx context.Context, tx pgx.Tx) error {
			return s.tickets.WithRiskWorkflowAuthorizationTx(ctx, tx, c.ActorID, []ticket.RiskWorkflowRoute{{Path: path, Method: "POST"}}, func(ctx context.Context) error {
				if err := s.tickets.CheckRiskWorkflowReadTx(ctx, tx, c.ActorID, c.TicketID); err != nil {
					return err
				}
				// Match the risk command operation-lock namespace, including cross-command
				// reuse of a key. It precedes all business locks and survives to commit.
				key, _ := json.Marshal([]any{c.ActorID, c.OperationID})
				if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,920092))`, string(key)); err != nil {
					return err
				}
				replay, found, err := s.assignmentReplay(ctx, tx, c, target, command)
				if err != nil {
					return err
				}
				if found {
					out = replay
					return nil
				}
				members, err := assignmentMembers(ctx, tx, c.TicketID)
				if err != nil {
					return err
				}
				if len(members) == 0 {
					return errcode.ErrNotFound
				}
				var scope *ticket.RiskWorkflowScope
				if target > 0 {
					scope, err = s.tickets.PrelockRiskWorkflowAssignmentTx(ctx, tx, c.TicketID, c.ActorID, memberOrgs(members)...)
				} else {
					scope, err = s.tickets.PrelockRiskWorkflowTx(ctx, tx, c.TicketID, c.ActorID, memberOrgs(members)...)
				}
				if err != nil {
					return err
				}
				if scope.DefinitionVersion() != input.DefinitionVersion || scope.Revision() != c.ExpectedTicketRevision {
					return errcode.ErrConflict
				}
				if scope.Status() != "assigned" {
					return errcode.ErrTicketInvalidTransition
				}
				if target == 0 && !scope.HasEdge("assigned", "in_progress") {
					return errcode.ErrTicketInvalidTransition
				}
				items, err := s.lockAssignmentMembers(ctx, tx, c, members, target > 0)
				if err != nil {
					return err
				}
				var handler int64
				if err = tx.QueryRow(ctx, `SELECT COALESCE(assigned_to,0) FROM tickets WHERE id=$1`, c.TicketID).Scan(&handler); err != nil {
					return err
				}
				if target == 0 && handler != c.ActorID {
					return errcode.ErrNoPermission
				}
				if target > 0 {
					if err = s.assignmentTarget(ctx, tx, c, target, members); err != nil {
						return err
					}
				}
				now := s.deps.Clock().Truncate(time.Microsecond)
				if now.IsZero() {
					return errcode.ErrServiceUnavailable
				}
				var tk *model.Ticket
				var generation int64
				if target > 0 {
					req := &ticket.RiskTicketReassignmentRequest{TicketID: c.TicketID, AssignedToUserID: target, ExpectedRevision: c.ExpectedTicketRevision, DefinitionVersion: input.DefinitionVersion}
					locked, e := s.tickets.LockRiskTicketReassignmentScopeTx(ctx, tx, req, c.ActorID, func(context.Context, pgx.Tx, int64, int64, int64, []repository.RiskTicketLinkBinding) error {
						return nil
					})
					if e != nil {
						return e
					}
					tk, err = s.tickets.ApplyRiskTicketReassignmentTx(ctx, tx, locked, func(ctx context.Context, tx pgx.Tx, before, after model.Ticket, actor int64) error {
						assignment, _, recordErr := s.deps.RecordAssignment(ctx, tx, after.ID, target, actor, now)
						if assignment != nil {
							generation = assignment.Generation
						}
						return recordErr
					})
				} else {
					innerKey := assignmentHash(c, 0)
					tk, err = s.tickets.ApplyRiskWorkflowTx(ctx, tx, scope, &model.TransitionTicketRequest{ID: c.TicketID, To: "in_progress", ExpectedRevision: decimal(c.ExpectedTicketRevision), DefinitionVersion: decimal(input.DefinitionVersion), CustomDataPatch: input.CustomDataPatch, Comment: input.Comment, IdempotencyKey: "risk-accept-" + hex.EncodeToString(innerKey[:])})
					if err != nil {
						return err
					}
					assignment, recorded, recordErr := s.deps.RecordAcceptance(ctx, tx, tk.ID, c.ActorID, now)
					if recordErr != nil {
						return recordErr
					}
					if assignment != nil {
						if !recorded {
							return errcode.ErrConflict
						}
						generation = assignment.Generation
					}
					target = c.ActorID
				}
				if err != nil {
					return err
				}
				out = AssignmentResult{TicketID: c.TicketID, TicketRevision: tk.Revision, DefinitionVersion: input.DefinitionVersion, AssignedToUserID: target, Generation: generation, Status: tk.Status, Items: items}
				// Acceptance's hash target is zero, independent of the resulting handler.
				hashTarget := target
				if command == "remediation_accept_assignment" {
					hashTarget = 0
				}
				return saveAssignmentReceipt(ctx, tx, c, hashTarget, command, out)
			})
		})
	})
	if err != nil {
		return AssignmentResult{}, err
	}
	return out, nil
}

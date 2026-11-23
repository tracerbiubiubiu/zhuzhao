package ticket

import (
	"context"
	"errors"
	"reflect"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// RiskTicketReassignmentRequest is an in-process command, not an HTTP DTO.
// This narrow port only replaces the handler of an already assigned ticket.
// Risk responsibility, historical creator/group owner and SLA stay unchanged.
type RiskTicketReassignmentRequest struct {
	TicketID, AssignedToUserID, ExpectedRevision, DefinitionVersion int64
}

// RiskTicketReassignmentScopeLocker must admit the actual dispatcher AND the
// fixed target handler using current credential/permission/target facts, locking
// group/family then all linked risks BEFORE any ticket lock. Binding owner is
// historical dispatch identity, never evidence of current risk responsibility.
// Keep all locks until outer commit; no commit, pool connection or external I/O.
type RiskTicketReassignmentScopeLocker func(context.Context, pgx.Tx, int64 /*ticket*/, int64 /*actor*/, int64 /*target*/, []repository.RiskTicketLinkBinding) error

// RiskTicketReassignmentHandoff is mandatory and runs after assignment/history
// and revision refresh, in the SAME transaction. Owned before/after snapshots
// and the real actor let the integrator record assignment generation, published
// reminder rule/deadline and suppress superseded intents atomically. Revision
// is a ticket concurrency token, NOT an assignment generation. Integrator owns
// generation allocation/controlled clock and the outer operation receipt.
// Acquire only later-order reminder rows, no risk/group locks, another pool
// connection, commit or external effects. Return every error to abort the owner.
type RiskTicketReassignmentHandoff func(context.Context, pgx.Tx, model.Ticket /*before*/, model.Ticket /*after*/, int64 /*actor*/) error

// LockedRiskTicketReassignment grants only this one fixed reassignment. It is
// exact-service/transaction/actor/target/version bound, and cannot authorize an
// ordinary Assign or a risk evidence action. Failed commands consume it too.
type LockedRiskTicketReassignment struct {
	service *Service
	tx      pgx.Tx
	actor   int64
	command RiskTicketReassignmentRequest
	binding []repository.RiskTicketLinkBinding
	used    atomic.Bool
}

func (s *Service) LockRiskTicketReassignmentScopeTx(ctx context.Context, tx pgx.Tx, req *RiskTicketReassignmentRequest, actor int64, lock RiskTicketReassignmentScopeLocker) (*LockedRiskTicketReassignment, error) {
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, err
	}
	return s.lockRiskTicketReassignmentScopeTx(ctx, tx, req, actor, lock)
}
func (s *Service) lockRiskTicketReassignmentScopeTx(ctx context.Context, tx pgx.Tx, req *RiskTicketReassignmentRequest, actor int64, lock RiskTicketReassignmentScopeLocker) (*LockedRiskTicketReassignment, error) {
	if req == nil || req.TicketID <= 0 || req.AssignedToUserID <= 0 || req.ExpectedRevision <= 0 || req.DefinitionVersion <= 0 || actor <= 0 || lock == nil {
		return nil, errcode.ErrInvalidParams
	}
	cmd := *req
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, cmd.TicketID)
	if err != nil {
		return nil, err
	}
	if len(binding) == 0 {
		return nil, errcode.ErrNoPermission
	}
	if err := lock(ctx, tx, cmd.TicketID, actor, cmd.AssignedToUserID, append([]repository.RiskTicketLinkBinding(nil), binding...)); err != nil {
		return nil, err
	}
	current, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, cmd.TicketID)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(current, binding) {
		return nil, errcode.ErrConcurrentModification
	}
	return &LockedRiskTicketReassignment{service: s, tx: tx, actor: actor, command: cmd, binding: binding}, nil
}

// ApplyRiskTicketReassignmentTx retains ticket Assign L2/L3 in addition to risk
// admission. Caller must propagate every error from its IAM callback and handle
// operation replay BEFORE a new scope. All returned data is provisional until
// outer acknowledged commit. No generic dispatch/post_actions are executed.
func (s *Service) ApplyRiskTicketReassignmentTx(ctx context.Context, tx pgx.Tx, scope *LockedRiskTicketReassignment, handoff RiskTicketReassignmentHandoff) (*model.Ticket, error) {
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, err
	}
	return s.applyRiskTicketReassignmentTx(ctx, tx, scope, handoff)
}
func (s *Service) applyRiskTicketReassignmentTx(ctx context.Context, tx pgx.Tx, scope *LockedRiskTicketReassignment, handoff RiskTicketReassignmentHandoff) (*model.Ticket, error) {
	if scope == nil || scope.service != s || scope.tx != tx || handoff == nil || !scope.used.CompareAndSwap(false, true) {
		return nil, errcode.ErrNoPermission
	}
	cmd := scope.command
	if err := s.authorizeCheck(ctx, scope.actor, "assign", strconv.FormatInt(cmd.TicketID, 10)); err != nil {
		return nil, err
	}
	before, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, cmd.TicketID)
	if err != nil {
		return nil, err
	}
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, cmd.TicketID)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(binding, scope.binding) {
		return nil, errcode.ErrConcurrentModification
	}
	if err := validateRiskTicketActionBinding(before, binding); err != nil {
		return nil, err
	}
	if before.Revision != cmd.ExpectedRevision || *before.DefinitionVersion != cmd.DefinitionVersion {
		return nil, errcode.ErrConcurrentModification
	}
	if before.Status == StatusClosed {
		return nil, errcode.ErrTicketAlreadyClosed
	}
	if before.Status != StatusAssigned {
		return nil, errcode.ErrTicketInvalidTransition
	}
	if *before.AssignedTo == cmd.AssignedToUserID {
		return nil, errcode.ErrConflict
	}
	_, ref, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, before.TypeCode, before.DefinitionVersion)
	if err != nil {
		return nil, err
	}
	// Read the target on the exact supplied transaction; the IAM barrier keeps
	// enabled/deleted facts stable. Target eligibility is additionally risk-scoped.
	var status int
	err = tx.QueryRow(ctx, `SELECT status FROM users WHERE id=$1 AND deleted_at IS NULL`, cmd.AssignedToUserID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrUserNotFound
	}
	if err != nil {
		return nil, err
	}
	if status != 1 {
		return nil, errcode.ErrUserDisabled
	}
	after := cloneTicketSnapshot(before)
	target := cmd.AssignedToUserID
	after.AssignedTo = &target
	after.AssigneeName, after.AssigneeNo = "", ""
	after.DefinitionRef = ref
	if err := s.ticketRepo.UpdateAssignedToTx(ctx, tx, before.ID, &target); err != nil {
		return nil, err
	}
	from, to := strconv.FormatInt(*before.AssignedTo, 10), strconv.FormatInt(target, 10)
	if err := s.ticketRepo.CreateEventTx(ctx, tx, &model.TicketEvent{TicketID: before.ID, UserID: scope.actor, Action: EventAssigned, FromValue: from, ToValue: to}); err != nil {
		return nil, err
	}
	if err := s.ticketRepo.RefreshRiskDispatchTicketTx(ctx, tx, &after); err != nil {
		return nil, err
	}
	if err := handoff(ctx, tx, cloneTicketSnapshot(before), cloneTicketSnapshot(&after), scope.actor); err != nil {
		return nil, err
	}
	eventTicket := cloneTicketSnapshot(&after)
	actor := scope.actor
	if err := repository.AfterIAMCommit(ctx, func() error {
		s.emitTicketEvent(&eventTicket, domainevent.DomainEvent{BizType: domainevent.BizTicket, Action: domainevent.TicketAssigned, BizID: strconv.FormatInt(eventTicket.ID, 10), ActorID: actor, Payload: domainevent.Payload{"from_assignee": from, "to_assignee": to, "status": eventTicket.Status}, Timestamp: time.Now()})
		return nil
	}); err != nil {
		return nil, err
	}
	return &after, nil
}

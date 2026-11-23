package ticket

import (
	"context"
	"crypto/sha256"
	"encoding/json"
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

// RiskTicketScopeLocker must lock/revalidate group/family then all linked risks,
// including credential, risk permission, lifecycle, versions and link identity.
// Current handling/retest responsibility MUST come from the locked risk facts.
// Group owner/owner_at_link and ticket creator are historical facts; neither
// they nor the current ticket assignee proves current risk review authority.
// It runs BEFORE any ticket lock. Return every error to the owning IAM guard.
// Binding is a copied observation, never proof of risk authorization or locks.
type RiskTicketScopeLocker func(context.Context, pgx.Tx, int64, int64, []repository.RiskTicketLinkBinding) error

// RiskTicketEvidenceGuard checks the normalized candidate and performs risk-side
// evidence/conclusion writes within the SAME transaction. It may use only the
// scope already locked by RiskTicketScopeLocker: no new group/risk/link locks,
// no commit, no pool connection, and no external side effects. Called before
// file locks and ticket mutation. Any failure must abort the outer transaction.
type RiskTicketEvidenceGuard func(context.Context, pgx.Tx, *model.Ticket, string, []repository.RiskTicketLinkBinding) error

// LockedRiskTicketAction is opaque, one-use and exact service/tx/actor/request
// bound. It carries no generic Close/Transition capability and cannot be supplied
// to either ordinary endpoint. Allocate it before locking any ticket rows.
type LockedRiskTicketAction struct {
	service *Service
	tx      pgx.Tx
	actor   int64
	command evidenceTransitionRequest
	binding []repository.RiskTicketLinkBinding
	used    atomic.Bool
}

func riskLifecycleAction(action string) bool {
	switch action {
	case "close", "transition", "assign", "delete":
		return true
	default:
		return false
	}
}

func (s *Service) admitOrdinaryRiskLifecycleTx(ctx context.Context, tx pgx.Tx, action string, actor int64, ids ...int64) error {
	if !riskLifecycleAction(action) {
		return nil
	}
	for _, id := range ids {
		if err := s.authorizeCheck(ctx, actor, action, strconv.FormatInt(id, 10)); err != nil {
			return err
		}
	}
	return s.rejectRiskLifecycleCommandTx(ctx, tx, action, ids...)
}

func riskTicketActionHash(original [sha256.Size]byte, binding []repository.RiskTicketLinkBinding) [sha256.Size]byte {
	raw, _ := json.Marshal(struct {
		Contract string
		Command  [sha256.Size]byte
		Binding  []repository.RiskTicketLinkBinding
	}{"risk.ticket.evidence.v1", original, binding})
	return sha256.Sum256(raw)
}

// Ordinary state writers have no risk scope. After the ticket lock they may
// only probe active links without acquiring any predecessor risk/group locks.
func (s *Service) rejectRiskLifecycleCommandTx(ctx context.Context, tx pgx.Tx, action string, ticketIDs ...int64) error {
	if !riskLifecycleAction(action) {
		return nil
	}
	for _, ticketID := range ticketIDs {
		binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, ticketID)
		if err != nil {
			return err
		}
		if len(binding) > 0 {
			return errcode.ErrNoPermission
		}
	}
	return nil
}

// LockRiskTicketActionScopeTx is a narrow in-process port, not an HTTP route.
// Caller owns IAM admission/fresh risk authorization and must not hold a ticket
// lock yet. Only versioned evidence actions on active personal bindings qualify.
func (s *Service) LockRiskTicketActionScopeTx(ctx context.Context, tx pgx.Tx, req *model.TransitionTicketRequest, actor int64, lock RiskTicketScopeLocker) (*LockedRiskTicketAction, error) {
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, err
	}
	return s.lockRiskTicketActionScopeTx(ctx, tx, req, actor, lock)
}

func (s *Service) lockRiskTicketActionScopeTx(ctx context.Context, tx pgx.Tx, req *model.TransitionTicketRequest, actor int64, lock RiskTicketScopeLocker) (*LockedRiskTicketAction, error) {
	if req == nil || req.ID <= 0 || actor <= 0 || lock == nil {
		return nil, errcode.ErrInvalidParams
	}
	cmd, err := parseEvidenceTransitionRequest(req)
	if err != nil {
		return nil, err
	}
	if !cmd.versioned() || cmd.to == "" {
		return nil, errcode.ErrInvalidParams
	}
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, cmd.ticketID)
	if err != nil {
		return nil, err
	}
	if len(binding) == 0 {
		return nil, errcode.ErrNoPermission
	}
	if err := lock(ctx, tx, cmd.ticketID, actor, append([]repository.RiskTicketLinkBinding(nil), binding...)); err != nil {
		return nil, err
	}
	current, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, cmd.ticketID)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(binding, current) {
		return nil, errcode.ErrConcurrentModification
	}
	cmd.hash = riskTicketActionHash(cmd.hash, binding)
	return &LockedRiskTicketAction{service: s, tx: tx, actor: actor, command: cmd, binding: binding}, nil
}

func validateRiskTicketActionBinding(t *model.Ticket, binding []repository.RiskTicketLinkBinding) error {
	if t == nil || len(binding) == 0 || t.DefinitionVersion == nil || *t.DefinitionVersion <= 0 || t.CreatedBy <= 0 || t.AssignedTo == nil || *t.AssignedTo <= 0 {
		return errcode.ErrConcurrentModification
	}
	group := binding[0].GroupID
	for _, b := range binding {
		if b.LinkID <= 0 || b.RiskID <= 0 || b.GroupID <= 0 || b.Revision <= 0 || !b.GroupActive || b.GroupID != group || b.OwnerUserID <= 0 || b.OwnerUserID != binding[0].OwnerUserID || b.OwnerAtLink != b.OwnerUserID || b.TargetOrgID != t.OrgID || b.TypeCode != t.TypeCode || b.DefinitionVersion != *t.DefinitionVersion {
			return errcode.ErrConcurrentModification
		}
	}
	return nil
}

// ApplyRiskTicketActionTx locks the ticket only after the risk scope is fixed,
// rechecks its active binding and original ticket authorization, then reuses the
// published versioned evidence kernel (G2/condition/validator/SLA/history/receipt).
// Result is provisional until the owning IAM transaction acknowledges commit.
func (s *Service) ApplyRiskTicketActionTx(ctx context.Context, tx pgx.Tx, scope *LockedRiskTicketAction, guard RiskTicketEvidenceGuard) (*model.Ticket, error) {
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, err
	}
	if scope == nil || scope.service != s || scope.tx != tx || guard == nil || !scope.used.CompareAndSwap(false, true) {
		return nil, errcode.ErrNoPermission
	}
	return s.applyRiskTicketActionTx(ctx, tx, scope, guard)
}

func (s *Service) applyRiskTicketActionTx(ctx context.Context, tx pgx.Tx, scope *LockedRiskTicketAction, guard RiskTicketEvidenceGuard) (*model.Ticket, error) {
	cmd := scope.command
	if err := s.authorizeCheck(ctx, scope.actor, "transition", strconv.FormatInt(cmd.ticketID, 10)); err != nil {
		return nil, err
	}
	before, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, cmd.ticketID)
	if err != nil {
		return nil, err
	}
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, cmd.ticketID)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(binding, scope.binding) {
		return nil, errcode.ErrConcurrentModification
	}
	if err := validateRiskTicketActionBinding(before, binding); err != nil {
		return nil, err
	}
	snapshot, ref, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, before.TypeCode, before.DefinitionVersion)
	if err != nil {
		return nil, err
	}
	replay, err := s.findEvidenceTransitionReplayTx(ctx, tx, cmd, scope.actor)
	if err != nil {
		return nil, err
	}
	if replay != nil {
		_, originalRef, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, before.TypeCode, &replay.DefinitionVersion)
		if err != nil {
			return nil, err
		}
		return ticketFromEvidenceReceipt(before.ID, originalRef, *replay), nil
	}
	_, err = s.checkTransitionCondition(ctx, before, cmd.to, scope.actor, &snapshot.Type)
	if err != nil {
		return nil, err
	}
	result, err := s.applyEvidenceTransitionGuardedTx(ctx, tx, cmd, before, snapshot, scope.actor, func(candidate *model.Ticket) error {
		return guard(ctx, tx, candidate, cmd.to, append([]repository.RiskTicketLinkBinding(nil), binding...))
	})
	if err != nil {
		return nil, err
	}
	out := ticketFromEvidenceReceipt(before.ID, ref, result)
	// General post_actions are not replayed under a risk scope. A successor risk
	// action must acquire its own current scope in its own admitted transaction.
	eventTicket := cloneTicketSnapshot(before)
	eventTicket.Status = cmd.to
	if err := repository.AfterIAMCommit(ctx, func() error {
		if before.Status != cmd.to {
			s.clearReminderWatermark(context.Background(), before.ID, before.Status)
		}
		s.emitTicketEvent(&eventTicket, domainevent.DomainEvent{BizType: domainevent.BizTicket, Action: domainevent.TicketTransitioned, BizID: strconv.FormatInt(before.ID, 10), ActorID: scope.actor, Payload: domainevent.Payload{"title": before.Title, "human_key": before.HumanKey, "to_assignee": strconv.FormatInt(*before.AssignedTo, 10), "type_code": before.TypeCode, "from_status": before.Status, "to_status": cmd.to, "status": cmd.to}, Timestamp: time.Now()})
		return nil
	}); err != nil {
		return nil, err
	}
	return out, nil
}

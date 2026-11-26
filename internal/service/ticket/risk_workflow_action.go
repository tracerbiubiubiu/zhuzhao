package ticket

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strconv"
	"sync/atomic"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// RiskWorkflowScope records the ticket's pinned rule and personal binding after
// type/org prelocks, before the risk kernel acquires group/risk/ticket/link locks.
// It grants no risk action; only the trusted kernel adapter may consume it after
// validating current risk responsibility on that exact transaction.
type RiskWorkflowScope struct {
	service        *Service
	tx             pgx.Tx
	actor          int64
	before         model.Ticket
	binding        []repository.RiskTicketLinkBinding
	sm             *StateMachine
	definition     *model.TicketDefinitionSnapshot
	definitionHash string
	retestReview   bool
	used           atomic.Bool
}

func (p *RiskWorkflowScope) Status() string {
	if p == nil {
		return ""
	}
	return p.before.Status
}
func (p *RiskWorkflowScope) DefinitionVersion() int64 {
	if p == nil || p.before.DefinitionVersion == nil {
		return 0
	}
	return *p.before.DefinitionVersion
}
func (p *RiskWorkflowScope) Revision() int64 {
	if p == nil {
		return 0
	}
	return p.before.Revision
}

func (p *RiskWorkflowScope) DefinitionHash() string {
	if p == nil {
		return ""
	}
	return p.definitionHash
}
func (p *RiskWorkflowScope) Allows(to string) bool {
	return p != nil && p.sm != nil && p.sm.CanTransition(p.before.Status, to)
}

// HasEdge lets the trusted adapter validate its complete fixed profile against
// the actual publication, including a partial-review self edge, before writes.
func (p *RiskWorkflowScope) HasEdge(from, to string) bool {
	return p != nil && p.sm != nil && p.sm.CanTransition(from, to)
}

// PrelockRiskWorkflowTx refreshes ticket read/transition authority and locks the
// type then all live organizations in ascending ID order. organizationIDs are
// trusted observations of current risk management organizations; the adapter
// must revalidate them under its subsequent risk locks. Duplicate IDs collapse.
// It never locks a ticket/group/risk/link or writes facts.
// The returned rule is the ticket's actual immutable publication, independent
// of later draft edits or the type's current publication pointer. Invoke inside
// WithRiskWorkflowAuthorizationTx on the kernel's exact IAM savepoint context.
func (s *Service) PrelockRiskWorkflowTx(ctx context.Context, tx pgx.Tx, ticketID, actor int64, organizationIDs ...int64) (*RiskWorkflowScope, error) {
	return s.prelockRiskWorkflowTx(ctx, tx, ticketID, actor, false, organizationIDs...)
}

func (s *Service) prelockRiskWorkflowTx(ctx context.Context, tx pgx.Tx, ticketID, actor int64, handoff bool, organizationIDs ...int64) (*RiskWorkflowScope, error) {
	if s == nil || s.ticketRepo == nil || s.orgRepo == nil || ticketID <= 0 || actor <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	for _, id := range organizationIDs {
		if id <= 0 {
			return nil, errcode.ErrInvalidParams
		}
	}
	if err := s.CheckRiskWorkflowReadTx(ctx, tx, actor, ticketID); err != nil {
		return nil, err
	}
	purpose, _ := middleware.IAMRequestFromContext(ctx)
	before, err := s.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	var code string
	err = tx.QueryRow(ctx, `SELECT code FROM ticket_types WHERE code=$1 FOR SHARE`, before.TypeCode).Scan(&code)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrTicketTypeNotFound
	}
	if err != nil {
		return nil, err
	}
	snapshot, ref, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, before.TypeCode, before.DefinitionVersion)
	if err != nil {
		return nil, err
	}
	if before.OrgID <= 0 {
		return nil, errcode.ErrInvalidParams
	}
	orgs := append([]int64{before.OrgID}, organizationIDs...)
	sort.Slice(orgs, func(i, j int) bool { return orgs[i] < orgs[j] })
	for i, id := range orgs {
		if i > 0 && orgs[i-1] == id {
			continue
		}
		if _, err := s.orgRepo.FindByIDForShareTx(ctx, tx, id); err != nil {
			return nil, err
		}
	}
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	if err := validateRiskTicketActionBinding(before, binding); err != nil {
		return nil, err
	}
	retest := !handoff && purpose.Path == riskRetestReviewPath && purpose.Method == "POST" &&
		before.Status == StatusPendingVerify && binding[0].RetestReviewerID == actor
	if !handoff {
		roles, err := s.freshCommandRoles(ctx, actor, purpose.Model, ticketPolicyRoute{path: "/api/v1/tickets/transition", method: "POST"})
		if err != nil {
			return nil, err
		}
		current := context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
		if err := s.authorizeCheck(current, actor, "transition", strconv.FormatInt(ticketID, 10)); err != nil && !(retest && errors.Is(err, errcode.ErrNoPermission)) {
			return nil, err
		}
	}
	sm, err := FromTicketType(&snapshot.Type)
	if err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	p := &RiskWorkflowScope{service: s, tx: tx, actor: actor, before: cloneTicketSnapshot(before), binding: binding, sm: sm, definition: snapshot,
		definitionHash: ref.DefinitionHash, retestReview: retest}
	if handoff {
		p.used.Store(true)
	}
	return p, nil
}

// ApplyRiskWorkflowTx runs only after the kernel has locked/revalidated the
// whole current group/risk/ticket/link scope and written its provisional effects.
// No predecessor lock is acquired here. The request consumes an actual published
// edge with ordinary ticket condition/evidence/history/SLA/receipt semantics.
// File/richtext collection, embedded comment files and general post-actions stay
// closed until their effects can join the risk command's exact evidence scope.
// Return all errors to the kernel's savepoint owner. No route is registered.
func (s *Service) ApplyRiskWorkflowTx(ctx context.Context, tx pgx.Tx, p *RiskWorkflowScope, req *model.TransitionTicketRequest) (*model.Ticket, error) {
	if s == nil || s.ticketRepo == nil {
		return nil, errcode.ErrInvalidParams
	}
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, err
	}
	proof, ok := ctx.Value(riskWorkflowAuthorizationKey{}).(riskWorkflowAuthorization)
	if p == nil || p.service != s || p.tx != tx || !ok || proof.service != s || proof.tx != tx || proof.actor != p.actor {
		return nil, errcode.ErrNoPermission
	}
	if req == nil || req.ID != p.before.ID {
		return nil, errcode.ErrInvalidParams
	}
	cmd, err := parseEvidenceTransitionRequest(req)
	if err != nil {
		return nil, err
	}
	if !cmd.versioned() || cmd.version != p.DefinitionVersion() || cmd.revision != p.Revision() {
		return nil, errcode.ErrConcurrentModification
	}
	if err := p.checkSupportedEdge(cmd.to); err != nil {
		return nil, err
	}
	if len(ExtractFileRefs(cmd.comment)) > 0 {
		return nil, errcode.ErrServiceUnavailable
	}
	if !p.used.CompareAndSwap(false, true) {
		return nil, errcode.ErrNoPermission
	}
	// Link revisions legitimately change inside this same kernel command. Keep
	// the original topology, but hash and execute against the now-locked revision.
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, req.ID)
	if err != nil {
		return nil, err
	}
	if !sameRiskWorkflowTopology(p.binding, binding) {
		return nil, errcode.ErrConcurrentModification
	}
	cmd.hash = riskTicketActionHash(cmd.hash, binding)
	if p.retestReview && (!riskRetestEdge(p.before.Status, cmd.to) || binding[0].RetestReviewerID != p.actor) {
		return nil, errcode.ErrNoPermission
	}
	scope := &LockedRiskTicketAction{service: s, tx: tx, actor: p.actor, command: cmd, binding: binding, retestReview: p.retestReview}
	return s.applyRiskTicketActionTx(ctx, tx, scope, func(_ context.Context, _ pgx.Tx, candidate *model.Ticket, _ string, _ []repository.RiskTicketLinkBinding) error {
		if candidate.TypeCode != p.before.TypeCode || candidate.OrgID != p.before.OrgID || candidate.Status != p.before.Status || candidate.DefinitionVersion == nil || *candidate.DefinitionVersion != p.DefinitionVersion() {
			return errcode.ErrConcurrentModification
		}
		return nil
	})
}

// checkSupportedEdge is shared by read-only workflow eligibility and execution.
func (p *RiskWorkflowScope) checkSupportedEdge(to string) error {
	if !p.Allows(to) {
		return errcode.ErrTicketInvalidTransition
	}
	if p.definition == nil {
		return errcode.ErrServiceUnavailable
	}
	meta, err := parseTransitionMeta(p.definition.Type.TransitionMeta, p.before.Status, to)
	if err != nil || (meta != nil && len(meta.PostActions) > 0) {
		return errcode.ErrServiceUnavailable
	}
	collect, err := transitionCollectFields(p.definition.Type.TransitionMeta, p.before.Status, to)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	for _, item := range collect {
		for _, field := range p.definition.Fields {
			if field != nil && field.FieldKey == item.Key && (field.FieldType == "file" || field.FieldType == "richtext") {
				return errcode.ErrServiceUnavailable
			}
		}
	}
	return nil
}

func sameRiskWorkflowTopology(before, after []repository.RiskTicketLinkBinding) bool {
	if len(before) != len(after) {
		return false
	}
	for i, b := range before {
		a := after[i]
		b.Revision = a.Revision
		if !reflect.DeepEqual(b, a) {
			return false
		}
	}
	return true
}

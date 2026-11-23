package ticket

import (
	"context"
	"encoding/hex"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// RiskDispatchCreatePreparation is immutable, service-owned preflight data.
// It carries no authority to dispatch a risk or choose its current owner.
type RiskDispatchCreatePreparation struct {
	service     *Service
	prepared    *preparedTicketCreate
	remediation bool
}

// LockedRiskDispatchCreate is valid only on the exact guarded transaction that
// locked type/template then org. Use once, after group/family and risk locks.
type LockedRiskDispatchCreate struct {
	service     *Service
	tx          pgx.Tx
	prepared    *preparedTicketCreate
	ticket      *model.Ticket
	remediation bool
	used        atomic.Bool
}

// ActorID is the authenticated dispatcher bound to this transaction.
func (c *LockedRiskDispatchCreate) ActorID() int64 {
	return c.prepared.actor
}

// RiskDispatchLinkWriter writes the risk link on the supplied transaction, after
// ticket/history and before file locks. It must propagate errors, acquire no
// earlier-order locks, and perform no commit or external side effects.
type RiskDispatchLinkWriter func(context.Context, pgx.Tx, int64) error

func validateRiskDispatchCreate(req *model.CreateTicketRequest, actor int64) error {
	if req == nil || actor <= 0 || req.TypeCode == "" || req.OrgID <= 0 || strings.TrimSpace(req.Title) == "" || req.AssignedTo != nil {
		return errcode.ErrInvalidParams
	}
	version, err := strconv.ParseInt(req.DefinitionVersion, 10, 64)
	if err != nil || version <= 0 || strconv.FormatInt(version, 10) != req.DefinitionVersion || len(req.DefinitionHash) != 64 || strings.ToLower(req.DefinitionHash) != req.DefinitionHash {
		return errcode.ErrInvalidParams
	}
	if _, err := hex.DecodeString(req.DefinitionHash); err != nil {
		return errcode.ErrInvalidParams
	}
	return nil
}

// PrepareRiskDispatchCreate runs outside the owning transaction. The real
// dispatcher is used for fetches; no ticket:create route capability is added.
// The risk service must pre-authorize before calling and recheck inside its guard.
func (s *Service) PrepareRiskDispatchCreate(ctx context.Context, req *model.CreateTicketRequest, actorUserID int64) (*RiskDispatchCreatePreparation, error) {
	if err := validateRiskDispatchCreate(req, actorUserID); err != nil {
		return nil, err
	}
	// Providers and human-key allocation must never take another pool connection
	// while the admitting risk transaction is holding locks.
	if repository.IAMGuardActive(ctx) {
		return nil, errcode.ErrInvalidParams
	}
	summary := *req
	summary.Title = model.RiskRemediationTicketTitle
	summary.Description = model.RiskRemediationTicketDescription
	p, err := s.prepareTicketCreate(ctx, &summary, actorUserID)
	if err != nil {
		return nil, err
	}
	return &RiskDispatchCreatePreparation{service: s, prepared: p}, nil
}

// PrepareRiskRemediationDispatchCreate opts into the fixed gate-on profile.
// The ordinary risk bridge keeps its existing definition contract when disabled.
func (s *Service) PrepareRiskRemediationDispatchCreate(ctx context.Context, req *model.CreateTicketRequest, actorUserID int64) (*RiskDispatchCreatePreparation, error) {
	p, err := s.PrepareRiskDispatchCreate(ctx, req, actorUserID)
	if err != nil {
		return nil, err
	}
	if err := ValidateRiskDispatchDefinition(p.prepared.ttype, p.prepared.fields); err != nil {
		return nil, err
	}
	p.remediation = true
	return p, nil
}

// LockRiskDispatchCreateTx must precede the risk service's group/family and risk
// locks. Risk authorization and target approval remain in that service.
func (s *Service) LockRiskDispatchCreateTx(ctx context.Context, tx pgx.Tx, p *RiskDispatchCreatePreparation) (*LockedRiskDispatchCreate, error) {
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, err
	}
	if p == nil || p.service != s || p.prepared == nil {
		return nil, errcode.ErrInvalidParams
	}
	ticket, err := s.lockPreparedCreateTx(ctx, tx, p.prepared, false)
	if err != nil {
		return nil, err
	}
	prepared := p.prepared
	if p.remediation {
		snapshot, _, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, ticket.TypeCode, ticket.DefinitionVersion)
		if err != nil {
			return nil, err
		}
		if err := ValidateRiskDispatchDefinition(&snapshot.Type, snapshot.Fields); err != nil {
			return nil, err
		}
		// All later validation and writes use the exact immutable pin read on
		// this transaction, never a current draft or a preparation-owned field.
		copy := *prepared
		copy.ttype = &snapshot.Type
		copy.fields = snapshot.Fields
		prepared = &copy
	}
	return &LockedRiskDispatchCreate{service: s, tx: tx, prepared: prepared, ticket: ticket, remediation: p.remediation}, nil
}

// CreateRiskDispatchTicketTx persists a risk ticket initially assigned to its
// individual owner through the bound published open>assigned edge. The
// caller has just revalidated owner provenance and approved targets under risk
// locks. Every error must abort the outer IAM callback. Returned data is still
// provisional until that callback's transaction commits successfully.
func (s *Service) CreateRiskDispatchTicketTx(ctx context.Context, tx pgx.Tx, locked *LockedRiskDispatchCreate, ownerUserID int64, link RiskDispatchLinkWriter) (*model.Ticket, error) {
	if err := s.ticketRepo.RequireGuardedCreateTx(ctx, s.db, tx); err != nil {
		return nil, err
	}
	if locked == nil || locked.service != s || locked.tx != tx || locked.prepared == nil || locked.ticket == nil || ownerUserID <= 0 || link == nil {
		return nil, errcode.ErrInvalidParams
	}
	if !locked.used.CompareAndSwap(false, true) {
		return nil, errcode.ErrConcurrentModification
	}
	// The IAM barrier keeps activation/deletion stable through commit. A user ID
	// is required; departments never become a ticket creator or grant access.
	if err := s.validateAssignTargetUser(ctx, ownerUserID); err != nil {
		return nil, err
	}
	ticket := locked.ticket
	if locked.remediation {
		if err := s.validateRiskDispatchInitialCandidate(ctx, tx, locked.prepared, ticket, ownerUserID); err != nil {
			return nil, err
		}
	}
	ticket.CreatedBy = ownerUserID
	if err := s.writeInitialRiskDispatchTx(ctx, tx, locked.prepared, ticket, ownerUserID, link); err != nil {
		return nil, err
	}
	// Take an owned event snapshot: callers cannot alter a queued notification by
	// mutating the provisional result. The guard releases it only on known commit.
	snapshot := riskDispatchNotificationSnapshot(ticket)
	actor := locked.prepared.actor
	if err := repository.AfterIAMCommit(ctx, func() error {
		s.emitTicketEvent(&snapshot, domainevent.DomainEvent{
			BizType: domainevent.BizTicket, Action: domainevent.TicketCreated,
			BizID: strconv.FormatInt(snapshot.ID, 10), ActorID: actor,
			Payload: domainevent.Payload{"status": snapshot.Status}, Timestamp: time.Now(),
		})
		s.emitTicketEvent(&snapshot, domainevent.DomainEvent{
			BizType: domainevent.BizTicket, Action: domainevent.TicketAssigned,
			BizID: strconv.FormatInt(snapshot.ID, 10), ActorID: actor,
			Payload: domainevent.Payload{"from_assignee": "", "to_assignee": strconv.FormatInt(ownerUserID, 10),
				"title": snapshot.Title, "type_code": snapshot.TypeCode, "priority": snapshot.Priority, "status": snapshot.Status},
			Timestamp: time.Now(),
		})
		return nil
	}); err != nil {
		return nil, err
	}
	return ticket, nil
}

func riskDispatchNotificationSnapshot(ticket *model.Ticket) model.Ticket {
	return cloneTicketSnapshot(ticket)
}

// validateRiskDispatchInitialCandidate must finish before ticket/history INSERTs.
func (s *Service) validateRiskDispatchInitialCandidate(ctx context.Context, tx pgx.Tx, p *preparedTicketCreate, ticket *model.Ticket, owner int64) error {
	candidate := cloneTicketSnapshot(ticket)
	candidate.CreatedBy = owner
	// Creation recomputes formulas and expands dictionary options before its
	// first edge. Preview that same candidate without mutating reusable fields.
	fields := make([]*model.TicketTypeField, len(p.fields))
	for i, field := range p.fields {
		copy := *field
		copy.FieldOptions = append([]byte(nil), field.FieldOptions...)
		fields[i] = &copy
	}
	data, err := s.validateCustomData(ctx, tx, p.req.TypeCode, candidate.CustomData, nil, fields)
	if err != nil {
		return err
	}
	candidate.CustomData = data
	_, err = s.checkTransitionMeta(ctx, &candidate, StatusAssigned, p.actor, p.ttype)
	return err
}

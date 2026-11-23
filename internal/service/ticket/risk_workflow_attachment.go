package ticket

import (
	"context"
	"errors"
	"reflect"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// RiskWorkflowAttachmentCommand is an opt-in storage adapter. The application
// must pass the same workflow enablement as the finish port and inject this
// adapter as the StorageHandler's TicketAuthorizer. Embedded ordinary methods
// retain their contracts; only AttachmentUploadCommand can request this scope.
type RiskWorkflowAttachmentCommand struct {
	*Service
	enabled bool
}

func NewRiskWorkflowAttachmentCommand(s *Service, enabled bool) *RiskWorkflowAttachmentCommand {
	return &RiskWorkflowAttachmentCommand{Service: s, enabled: enabled}
}

type riskAttachmentKey struct{}
type riskAttachmentRequest struct {
	service       *Service
	enabled       bool
	action        string
	actor, ticket int64
	file          model.FileObject
}

func (a *RiskWorkflowAttachmentCommand) AttachmentUploadCommand(ctx context.Context, action string, actor, ticketID int64, f *model.FileObject, write func(context.Context) error) error {
	if a == nil || a.Service == nil || f == nil || write == nil || f.Purpose != "ticket_attachment" || f.CreatedBy != actor || f.Bucket == "" || f.ObjectKey == "" || (action != "presign" && action != "confirm") {
		return errcode.ErrInvalidParams
	}
	// Direct callers never gain a new resource permission from this adapter.
	if _, ok := middleware.IAMRequestFromContext(ctx); !ok {
		return a.AttachmentCommand(ctx, action, actor, ticketID, write)
	}
	// Disabling workflow expansion must not erase an existing reservation pin.
	ctx = context.WithValue(ctx, riskAttachmentKey{}, riskAttachmentRequest{a.Service, a.enabled, action, actor, ticketID, *f})
	return a.AttachmentCommand(ctx, action, actor, ticketID, write)
}

type riskAttachmentScope struct {
	before   *model.Ticket
	binding  []repository.RiskTicketLinkBinding
	snapshot *model.TicketDefinitionSnapshot
}

// Lock predecessors BEFORE AttachmentCommand locks the ticket. In particular,
// risk.Assign can change visibility while holding only a risk row, so a ticket
// lock plus an unlocked risk read is insufficient. No external I/O runs here.
func (s *Service) prelockRiskAttachment(ctx context.Context, tx pgx.Tx, actor, ticketID int64) (*riskAttachmentScope, error) {
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actor || !purpose.SelfService || purpose.SessionEpoch == nil || *purpose.SessionEpoch < 1 || purpose.PATID != 0 {
		return nil, errcode.ErrNoPermission
	}
	roles, err := s.freshCommandRoles(ctx, actor, purpose.Model,
		ticketPolicyRoute{path: "/api/v1/risk-items/remediation-finish", method: "POST"},
		ticketPolicyRoute{path: "/api/v1/tickets/transition", method: "POST"},
		ticketPolicyRoute{path: "/api/v1/tickets/" + strconv.FormatInt(ticketID, 10), method: "GET"})
	if err != nil {
		return nil, err
	}
	ctx = context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
	for _, action := range []string{"read", "transition"} {
		if err := s.authorizeCheck(ctx, actor, action, strconv.FormatInt(ticketID, 10)); err != nil {
			return nil, err
		}
	}
	before, err := s.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return nil, err
	}
	if before.AssignedTo == nil || *before.AssignedTo != actor || before.Status != "in_progress" {
		return nil, errcode.ErrNoPermission
	}
	var code string
	if err := tx.QueryRow(ctx, `SELECT code FROM ticket_types WHERE code=$1 FOR SHARE`, before.TypeCode).Scan(&code); err != nil {
		return nil, err
	}
	snapshot, _, err := s.ticketRepo.BoundTicketDefinitionTx(ctx, tx, before.TypeCode, before.DefinitionVersion)
	if err != nil {
		return nil, err
	}
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	if err := validateRiskTicketActionBinding(before, binding); err != nil {
		return nil, err
	}
	// Include all observed management/affected organizations. If topology or risk
	// organization changes before its locks, reject rather than lock backwards.
	rows, err := tx.Query(ctx, `SELECT id FROM organizations WHERE id IN (
	 SELECT $2::bigint UNION SELECT g.management_org_id FROM risk_dispatch_groups g WHERE g.ticket_id=$1
	 UNION SELECT r.org_id FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id JOIN risk_dispatch_groups g ON g.id=l.group_id WHERE g.ticket_id=$1 AND l.ended_at IS NULL
	 UNION SELECT r.affected_org_id FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id JOIN risk_dispatch_groups g ON g.id=l.group_id WHERE g.ticket_id=$1 AND l.ended_at IS NULL
	) ORDER BY id`, ticketID, before.OrgID)
	if err != nil {
		return nil, err
	}
	orgs, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, err
	}
	for _, org := range orgs {
		if _, err := s.orgRepo.FindByIDForShareTx(ctx, tx, org); err != nil {
			return nil, err
		}
	}
	var group int64
	if err := tx.QueryRow(ctx, `SELECT id FROM risk_dispatch_groups WHERE id=$1 AND ticket_id=$2 AND ended_at IS NULL FOR SHARE`, binding[0].GroupID, ticketID).Scan(&group); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrNoPermission
		}
		return nil, err
	}
	current, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, ticketID)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(binding, current) {
		return nil, errcode.ErrConcurrentModification
	}
	ids := make([]int64, 0, len(binding))
	for _, b := range binding {
		ids = append(ids, b.RiskID)
	}
	rows, err = tx.Query(ctx, `SELECT id FROM risk_items WHERE id=ANY($1) ORDER BY id FOR SHARE`, ids)
	if err != nil {
		return nil, err
	}
	locked, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return nil, err
	}
	if len(locked) != len(binding) {
		return nil, errcode.ErrConcurrentModification
	}
	var orgDrift bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_items WHERE id=ANY($1) AND (NOT (org_id=ANY($2)) OR (affected_org_id IS NOT NULL AND NOT (affected_org_id=ANY($2)))))`, ids, orgs).Scan(&orgDrift); err != nil {
		return nil, err
	}
	if orgDrift {
		return nil, errcode.ErrConcurrentModification
	}
	for _, b := range binding {
		if _, _, _, err := repository.GetRiskItemVisible(ctx, tx, b.RiskID, actor); err != nil {
			if errors.Is(err, repository.ErrRiskNotFound) {
				return nil, errcode.ErrNoPermission
			}
			return nil, err
		}
	}
	return &riskAttachmentScope{before, binding, snapshot}, nil
}

func (s *Service) checkRiskAttachment(ctx context.Context, tx pgx.Tx, actor int64, tk *model.Ticket, p *riskAttachmentScope) error {
	if tk.AssignedTo == nil || *tk.AssignedTo != actor || tk.Status != "in_progress" {
		return errcode.ErrNoPermission
	}
	if tk.TypeCode != p.before.TypeCode || tk.OrgID != p.before.OrgID || !reflect.DeepEqual(tk.DefinitionVersion, p.before.DefinitionVersion) {
		return errcode.ErrConcurrentModification
	}
	binding, err := s.ticketRepo.ReadActiveRiskTicketBindingTx(ctx, tx, tk.ID)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(binding, p.binding) {
		return errcode.ErrConcurrentModification
	}
	if err := validateRiskTicketActionBinding(tk, binding); err != nil {
		return err
	}
	sm, err := FromTicketType(&p.snapshot.Type)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	for _, edge := range [][2]string{{"in_progress", "pending_verify"}, {"pending_verify", "pending_verify"}, {"pending_verify", "closed"}, {"pending_verify", "in_progress"}} {
		if !sm.CanTransition(edge[0], edge[1]) {
			return errcode.ErrServiceUnavailable
		}
	}
	meta, err := s.checkTransitionCondition(ctx, tk, "pending_verify", actor, &p.snapshot.Type)
	if err != nil {
		return err
	}
	if meta != nil && len(meta.PostActions) > 0 {
		return errcode.ErrServiceUnavailable
	}
	collect, err := transitionCollectFields(p.snapshot.Type.TransitionMeta, "in_progress", "pending_verify")
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	for _, c := range collect {
		for _, f := range p.snapshot.Fields {
			if f != nil && f.FieldKey == c.Key && (f.FieldType == "file" || f.FieldType == "richtext") {
				return errcode.ErrServiceUnavailable
			}
		}
	}
	// Finish supports rejected resubmission and resolved siblings. Only require
	// an actionable member; uploading does not run future evidence validators.
	var actionable bool
	err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM risk_ticket_links l JOIN risk_items r ON r.id=l.risk_id WHERE l.group_id=$1 AND l.ended_at IS NULL AND l.remediation_state NOT IN ('resolved','pending_verify') AND r.owner_user_id IS NOT NULL AND r.status IN ('remediating','pending_verify'))`, binding[0].GroupID).Scan(&actionable)
	if err != nil {
		return err
	}
	if !actionable {
		return errcode.ErrNoPermission
	}
	return nil
}

// Ticket precedes file locks, exactly as ConfirmAndBind. Reserve an original
// ticket on presign; confirm must check this even when ordinary update succeeds.
func checkAttachmentReservation(ctx context.Context, tx pgx.Tx, p riskAttachmentRequest, requirePin bool) error {
	var kind, id *string
	err := tx.QueryRow(ctx, `SELECT owner_kind,owner_id FROM file_objects WHERE bucket=$1 AND object_key=$2 AND created_by=$3 AND purpose='ticket_attachment' AND status='pending' FOR UPDATE`, p.file.Bucket, p.file.ObjectKey, p.actor).Scan(&kind, &id)
	if errors.Is(err, pgx.ErrNoRows) {
		return errcode.ErrFileAlreadyBound
	}
	if err != nil {
		return err
	}
	if kind == nil && id == nil && !requirePin {
		return nil
	}
	if kind == nil || id == nil || *kind != model.FileOwnerTicket || *id != strconv.FormatInt(p.ticket, 10) {
		return errcode.ErrNoPermission
	}
	return nil
}

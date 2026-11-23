package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const personalRiskBinding = `[{"link_id":3,"risk_id":7,"group_id":5,"revision":2,"owner_at_link":22,"owner_user_id":22,"target_org_id":4,"type_code":"incident","definition_version":3,"group_active":true}]`
const personalRiskDefinition = `{"type":{"code":"incident","states":["open","assigned","pending_verify","closed"],"transitions":{"open":["assigned"],"assigned":["pending_verify"],"pending_verify":["closed"]},"transition_meta":{"assigned>pending_verify":{"collect_fields":[{"key":"result","required":true}]}},"default_sla_hours":0,"has_custom_fields":false},"fields":[{"field_id":"1","type_code":"incident","field_key":"result","field_type":"input","required":false,"sort_order":0}]}`

// Database results, resource authorization and current risk facts are synthetic
// ports. Service admission wiring, immutable definition decoding, graph/condition
// and evidence validation run normally; this is not real IAM/risk acceptance.
type riskRolesTx struct {
	*riskEvidenceTx
	ticket     *model.Ticket
	binding    string
	definition string
}

func (tx *riskRolesTx) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	switch {
	case strings.Contains(q, "SET custom_data="):
		tx.steps = append(tx.steps, "ticket-write")
		require.Equal(tx.t, []any{int64(41), int64(5), int64(3), StatusPendingVerify, StatusPendingVerify}, args[:5])
		tx.data = append(json.RawMessage(nil), args[5].(json.RawMessage)...)
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*string) = StatusPendingVerify
			*d[1].(*int64) = 6
			*d[2].(*int64) = 3
			*d[3].(**time.Time) = tx.ticket.StatusChangedAt
			return nil
		})
	case strings.Contains(q, "to_regclass"):
		return bridgeRowFunc(func(d ...any) error { *d[0].(*bool) = true; *d[1].(*bool) = true; return nil })
	case strings.Contains(q, "FROM risk_ticket_links"):
		return bridgeRowFunc(func(d ...any) error { *d[0].(*json.RawMessage) = json.RawMessage(tx.binding); return nil })
	case strings.Contains(q, "FROM tickets WHERE id = $1 FOR UPDATE"):
		tx.steps = append(tx.steps, "ticket-lock")
		return bridgeRowFunc(func(d ...any) error {
			b := tx.ticket
			values := []any{b.ID, b.TypeCode, b.Title, b.Description, b.Priority, b.Status, b.CreatedBy, b.AssignedTo, b.OrgID, b.OrgPath, b.CustomData, b.SLADueAt, b.StatusChangedAt, b.EscalationSupervisor, b.CreatedAt, b.UpdatedAt, b.HumanKey, b.DefinitionVersion, b.Revision}
			for i, v := range values {
				reflect.ValueOf(d[i]).Elem().Set(reflect.ValueOf(v))
			}
			return nil
		})
	case strings.Contains(q, "FROM ticket_type_definitions"):
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*string) = strings.Repeat("a", 64)
			*d[1].(*json.RawMessage) = json.RawMessage(tx.definition)
			return nil
		})
	case strings.Contains(q, "FROM ticket_evidence_transition_receipts"):
		return bridgeRowFunc(func(...any) error { return pgx.ErrNoRows })
	default:
		return tx.riskEvidenceTx.QueryRow(ctx, q, args...)
	}
}
func riskRolesFixture(t *testing.T) (*Service, *riskRolesTx, *model.TransitionTicketRequest) {
	t.Helper()
	s, b, _, _ := riskEvidenceFixture(t)
	p := int64(33)
	b.CreatedBy = 22
	b.AssignedTo = &p
	b.OrgID = 4
	tx := &riskRolesTx{riskEvidenceTx: &riskEvidenceTx{t: t, revision: 5}, ticket: b, binding: personalRiskBinding, definition: personalRiskDefinition}
	s.registry = &riskActionRegistry{allowed: true}
	s.delegation = initialRiskDelegation{}
	req := &model.TransitionTicketRequest{ID: 41, To: StatusPendingVerify, ExpectedRevision: json.RawMessage(`"5"`), DefinitionVersion: json.RawMessage(`"3"`), IdempotencyKey: "synthetic-mup", CustomDataPatch: json.RawMessage(`{"result":"accepted"}`)}
	return s, tx, req
}
func riskRoleContext(actor int64) context.Context {
	return context.WithValue(context.Background(), ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: []string{"user"}})
}

// Reintroducing creator==assignee or group owner==creator breaks these cases.
// Risk responsibility is decided from current locked risk facts, independently
// of the handler P and historical creator/dispatch owner U.
func TestRiskActionMUPAndSuccessorReachCurrentRiskGuard(t *testing.T) {
	for _, tc := range []struct {
		name           string
		actor, creator int64
	}{
		{"P handles after M reassigned", 33, 22},
		{"U retests after P handles", 22, 22},
		{"current successor retests without rewriting creator", 44, 22},
		{"historical creator differs from dispatch owner", 44, 55},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, tx, req := riskRolesFixture(t)
			tx.ticket.CreatedBy = tc.creator
			ctx := riskRoleContext(tc.actor)
			scope, err := s.lockRiskTicketActionScopeTx(ctx, tx, req, tc.actor, func(_ context.Context, got pgx.Tx, _, actor int64, _ []repository.RiskTicketLinkBinding) error {
				require.Same(t, tx, got)
				require.Equal(t, tc.actor, actor)
				tx.steps = append(tx.steps, "current-risk-scope")
				return nil
			})
			require.NoError(t, err)
			stop := errors.New("synthetic stop at authorized current-risk evidence guard")
			_, err = s.applyRiskTicketActionTx(ctx, tx, scope, func(_ context.Context, got pgx.Tx, candidate *model.Ticket, _ string, _ []repository.RiskTicketLinkBinding) error {
				require.Same(t, tx, got)
				require.Equal(t, int64(33), *candidate.AssignedTo)
				require.Equal(t, tc.creator, candidate.CreatedBy)
				tx.steps = append(tx.steps, "risk-evidence-guard")
				return stop
			})
			require.ErrorIs(t, err, stop)
			require.Equal(t, []string{"current-risk-scope", "ticket-lock", "revision", "risk-evidence-guard"}, tx.steps)
			require.Equal(t, tc.creator, tx.ticket.CreatedBy)
			require.Equal(t, int64(33), *tx.ticket.AssignedTo)
		})
	}
}
func TestRiskActionCurrentScopeDeniesXAndFormerOwnerWithoutTicketWrite(t *testing.T) {
	for _, actor := range []int64{66, 22} {
		s, tx, req := riskRolesFixture(t)
		_, err := s.lockRiskTicketActionScopeTx(riskRoleContext(actor), tx, req, actor, func(_ context.Context, _ pgx.Tx, _, got int64, _ []repository.RiskTicketLinkBinding) error {
			require.Equal(t, actor, got)
			return errcode.ErrNoPermission // Current owner is successor 44.
		})
		require.ErrorIs(t, err, errcode.ErrNoPermission)
		require.Empty(t, tx.steps)
	}
}
func TestRiskActionRiskScopeCannotSupplyTicketTransitionPermission(t *testing.T) {
	s, tx, req := riskRolesFixture(t)
	ctx := riskRoleContext(44)
	scope, err := s.lockRiskTicketActionScopeTx(ctx, tx, req, 44, func(context.Context, pgx.Tx, int64, int64, []repository.RiskTicketLinkBinding) error { return nil })
	require.NoError(t, err)
	s.registry = &riskActionRegistry{allowed: false}
	_, err = s.applyRiskTicketActionTx(ctx, tx, scope, func(context.Context, pgx.Tx, *model.Ticket, string, []repository.RiskTicketLinkBinding) error {
		t.Fatal("must not reach guard")
		return nil
	})
	require.ErrorIs(t, err, errcode.ErrTicketNotFound)
	require.Empty(t, tx.steps)
}

func TestRiskActionScopeDoesNotBypassPublishedTransitionCondition(t *testing.T) {
	s, tx, req := riskRolesFixture(t)
	ctx := riskRoleContext(44)
	s.delegation = initialRiskDelegation{}
	tx.definition = strings.ReplaceAll(personalRiskDefinition, `"collect_fields":`, `"roles":["assignee"],"collect_fields":`)
	scope, err := s.lockRiskTicketActionScopeTx(ctx, tx, req, 44, func(context.Context, pgx.Tx, int64, int64, []repository.RiskTicketLinkBinding) error { return nil })
	require.NoError(t, err)
	_, err = s.applyRiskTicketActionTx(ctx, tx, scope, func(context.Context, pgx.Tx, *model.Ticket, string, []repository.RiskTicketLinkBinding) error {
		t.Fatal("condition must deny before evidence guard")
		return nil
	})
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.Equal(t, []string{"ticket-lock"}, tx.steps)
}

// A per-item review may keep the whole ticket pending. U or the current legal
// successor must be able to persist evidence/history/receipt while P handles it.
func TestRiskActionCurrentReviewerPersistsWithDifferentHandlerAndCreator(t *testing.T) {
	for _, actor := range []int64{22, 44} {
		t.Run(strconv.FormatInt(actor, 10), func(t *testing.T) {
			s, tx, req := riskRolesFixture(t)
			ctx := riskRoleContext(actor)
			tx.ticket.Status = StatusPendingVerify
			tx.receiptRows = 1
			tx.definition = strings.ReplaceAll(personalRiskDefinition, `"pending_verify":["closed"]`, `"pending_verify":["pending_verify","closed"]`)
			tx.definition = strings.ReplaceAll(tx.definition, `"assigned>pending_verify"`, `"pending_verify>pending_verify"`)
			scope, err := s.lockRiskTicketActionScopeTx(ctx, tx, req, actor, func(_ context.Context, got pgx.Tx, _, who int64, b []repository.RiskTicketLinkBinding) error {
				require.Same(t, tx, got)
				require.Equal(t, actor, who)
				require.Equal(t, int64(22), b[0].OwnerUserID)
				return nil
			})
			require.NoError(t, err)
			out, err := s.applyRiskTicketActionTx(ctx, tx, scope, func(_ context.Context, got pgx.Tx, candidate *model.Ticket, _ string, _ []repository.RiskTicketLinkBinding) error {
				require.Same(t, tx, got)
				require.Equal(t, int64(33), *candidate.AssignedTo)
				require.Equal(t, int64(22), candidate.CreatedBy)
				tx.steps = append(tx.steps, "current-reviewer-guard")
				return nil
			})
			require.NoError(t, err)
			require.Equal(t, []string{"ticket-lock", "revision", "current-reviewer-guard", "ticket-write", "event", "receipt"}, tx.steps)
			require.Equal(t, []int64{actor}, tx.eventActors)
			require.Equal(t, actor, tx.receiptActor)
			require.Equal(t, StatusPendingVerify, out.Status)
			require.Equal(t, int64(6), out.Revision)
			require.JSONEq(t, `{"result":"accepted","unknown":"retained"}`, string(tx.data))
			require.Equal(t, int64(22), tx.ticket.CreatedBy)
			require.Equal(t, int64(33), *tx.ticket.AssignedTo)
		})
	}
}

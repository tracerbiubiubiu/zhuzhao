package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type riskActionProbeTx struct {
	pgx.Tx
	raw string
	err error
}

func (tx *riskActionProbeTx) QueryRow(_ context.Context, q string, _ ...any) pgx.Row {
	return bridgeRowFunc(func(d ...any) error {
		if tx.err != nil {
			return tx.err
		}
		if strings.Contains(q, "to_regclass") {
			*d[0].(*bool) = true
			*d[1].(*bool) = true
			return nil
		}
		*d[0].(*json.RawMessage) = json.RawMessage(tx.raw)
		return nil
	})
}
func TestOrdinaryLifecycleRejectsActiveRiskLinks(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	for _, action := range []string{"close", "transition", "assign", "delete"} {
		t.Run(action, func(t *testing.T) {
			tx := &riskActionProbeTx{raw: `[{"link_id":3}]`}
			require.ErrorIs(t, s.rejectRiskLifecycleCommandTx(context.Background(), tx, action, 41), errcode.ErrNoPermission)
			tx.raw = `[]`
			require.NoError(t, s.rejectRiskLifecycleCommandTx(context.Background(), tx, action, 41))
			tx.err = errors.New("synthetic lookup failure")
			require.ErrorIs(t, s.rejectRiskLifecycleCommandTx(context.Background(), tx, action, 41), tx.err)
		})
	}
}
func TestRiskLifecycleDoesNotRejectUnrelatedCommands(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	tx := &riskActionProbeTx{err: errors.New("must not query")}
	for _, action := range []string{"comments", "notes", "tags", "update"} {
		require.NoError(t, s.rejectRiskLifecycleCommandTx(context.Background(), tx, action, 41))
	}
}
func TestRiskActionRejectsBareGuardAndLegacyRequest(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	_, err := s.LockRiskTicketActionScopeTx(context.Background(), nil, &model.TransitionTicketRequest{ID: 41, To: "closed"}, 22, nil)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	_, err = s.ApplyRiskTicketActionTx(context.Background(), nil, nil, nil)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
}
func TestRiskActionBindingMustMatchPinnedPersonalTicket(t *testing.T) {
	version, owner := int64(1), int64(22)
	tk := &model.Ticket{ID: 41, TypeCode: "incident", OrgID: 4, CreatedBy: owner, AssignedTo: &owner, DefinitionVersion: &version}
	binding := []repository.RiskTicketLinkBinding{{LinkID: 3, RiskID: 7, GroupID: 5, Revision: 2, OwnerAtLink: 22, OwnerUserID: 22, TargetOrgID: 4, TypeCode: "incident", DefinitionVersion: 1, GroupActive: true}}
	require.NoError(t, validateRiskTicketActionBinding(tk, binding))
	for _, mutate := range []func(*repository.RiskTicketLinkBinding){
		func(b *repository.RiskTicketLinkBinding) { b.OwnerAtLink = 9 },
		func(b *repository.RiskTicketLinkBinding) { b.OwnerUserID = 9 },
		func(b *repository.RiskTicketLinkBinding) { b.DefinitionVersion = 2 },
		func(b *repository.RiskTicketLinkBinding) { b.TargetOrgID = 9 },
		func(b *repository.RiskTicketLinkBinding) { b.TypeCode = "other" },
		func(b *repository.RiskTicketLinkBinding) { b.GroupActive = false },
		func(b *repository.RiskTicketLinkBinding) { b.Revision = 0 },
	} {
		changed := append([]repository.RiskTicketLinkBinding(nil), binding...)
		mutate(&changed[0])
		require.Error(t, validateRiskTicketActionBinding(tk, changed))
	}
	require.Error(t, validateRiskTicketActionBinding(tk, nil))
	tk.AssignedTo = nil
	require.Error(t, validateRiskTicketActionBinding(tk, binding))
}

type riskActionRegistry struct {
	resource.Registry
	allowed bool
	calls   int
}

func (r *riskActionRegistry) Authorize(_ context.Context, _ string, _ resource.AuthorizeRequest) (bool, error) {
	r.calls++
	return r.allowed, nil
}
func TestRiskLifecycleAuthorizationPrecedesBindingProbe(t *testing.T) {
	registry := &riskActionRegistry{}
	s := &Service{ticketRepo: repository.NewTicketRepo(nil), registry: registry}
	ctx := context.WithValue(context.Background(), ticketCommandRolesKey{}, ticketCommandRoles{actor: 22, roles: []string{"user"}})
	tx := &riskActionProbeTx{err: errors.New("must not query before authorization")}
	require.ErrorIs(t, s.admitOrdinaryRiskLifecycleTx(ctx, tx, "close", 22, 41), errcode.ErrTicketNotFound)
	require.Equal(t, 1, registry.calls)
	registry.allowed = true
	tx.err = nil
	tx.raw = `[{"link_id":3}]`
	require.ErrorIs(t, s.admitOrdinaryRiskLifecycleTx(ctx, tx, "transition", 22, 41), errcode.ErrNoPermission)
}
func TestRiskActionFingerprintCannotReplayOrdinaryReceipt(t *testing.T) {
	req := &model.TransitionTicketRequest{ID: 41, To: "closed", ExpectedRevision: json.RawMessage(`"2"`), DefinitionVersion: json.RawMessage(`"1"`), IdempotencyKey: "synthetic-op"}
	ordinary, err := parseEvidenceTransitionRequest(req)
	require.NoError(t, err)
	binding := []repository.RiskTicketLinkBinding{{LinkID: 3, Revision: 1}}
	first := riskTicketActionHash(ordinary.hash, binding)
	require.NotEqual(t, ordinary.hash, first)
	require.Equal(t, first, riskTicketActionHash(ordinary.hash, binding))
	binding[0].Revision++
	require.NotEqual(t, first, riskTicketActionHash(ordinary.hash, binding))
}

func TestRiskEvidenceCandidateCannotMutateTicketPointers(t *testing.T) {
	s, before, snapshot, cmd := riskEvidenceFixture(t)
	owner, supervisor := int64(22), int64(23)
	at := time.Unix(100, 0)
	before.AssignedTo = &owner
	before.EscalationSupervisor = &supervisor
	before.SLADueAt = &at
	before.StatusChangedAt = &at
	before.DefinitionRef = &model.TicketDefinitionRef{TypeCode: "incident", DefinitionVersion: 3}
	tx := &riskEvidenceTx{t: t, revision: 5, receiptRows: 1}
	sentinel := errors.New("synthetic guard rejects")
	_, err := s.applyEvidenceTransitionGuardedTx(context.Background(), tx, cmd, before, snapshot, 22, func(candidate *model.Ticket) error {
		*candidate.AssignedTo = 99
		*candidate.DefinitionVersion = 99
		*candidate.EscalationSupervisor = 99
		*candidate.SLADueAt = time.Time{}
		*candidate.StatusChangedAt = time.Time{}
		candidate.DefinitionRef.DefinitionVersion = 99
		return sentinel
	})
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, int64(22), *before.AssignedTo)
	require.Equal(t, int64(3), *before.DefinitionVersion)
	require.Equal(t, int64(23), *before.EscalationSupervisor)
	require.Equal(t, time.Unix(100, 0), *before.SLADueAt)
	require.Equal(t, time.Unix(100, 0), *before.StatusChangedAt)
	require.Equal(t, int64(3), before.DefinitionRef.DefinitionVersion)
}

type riskScopeTx struct {
	pgx.Tx
	t     *testing.T
	raws  []string
	steps []string
}

func (tx *riskScopeTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	require.NotContains(tx.t, strings.ToUpper(q), "FOR UPDATE")
	return bridgeRowFunc(func(d ...any) error {
		if strings.Contains(q, "to_regclass") {
			*d[0].(*bool) = true
			*d[1].(*bool) = true
			return nil
		}
		require.Equal(tx.t, int64(41), args[0])
		tx.steps = append(tx.steps, "binding")
		raw := tx.raws[0]
		if len(tx.raws) > 1 {
			tx.raws = tx.raws[1:]
		}
		*d[0].(*json.RawMessage) = json.RawMessage(raw)
		return nil
	})
}
func TestRiskActionScopeLocksBeforeTicketAndOwnsCommand(t *testing.T) {
	raw := `[{"link_id":3,"revision":1}]`
	tx := &riskScopeTx{t: t, raws: []string{raw}}
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	req := &model.TransitionTicketRequest{ID: 41, To: "closed", ExpectedRevision: json.RawMessage(`"2"`), DefinitionVersion: json.RawMessage(`"1"`), IdempotencyKey: "synthetic-op", CustomDataPatch: json.RawMessage(`{"x":1}`)}
	scope, err := s.lockRiskTicketActionScopeTx(context.Background(), tx, req, 22, func(_ context.Context, got pgx.Tx, id, actor int64, b []repository.RiskTicketLinkBinding) error {
		require.Same(t, tx, got)
		require.Equal(t, int64(41), id)
		require.Equal(t, int64(22), actor)
		tx.steps = append(tx.steps, "risk-lock")
		b[0].Revision = 99
		req.ID = 999
		req.To = "open"
		req.CustomDataPatch[5] = '9'
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"binding", "risk-lock", "binding"}, tx.steps)
	require.Equal(t, int64(41), scope.command.ticketID)
	require.Equal(t, "closed", scope.command.to)
	require.JSONEq(t, `{"x":1}`, string(scope.command.patch))
	require.Equal(t, int64(1), scope.binding[0].Revision)
}
func TestRiskActionScopeRejectsChangedBindingAndLockerFailure(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	req := &model.TransitionTicketRequest{ID: 41, To: "closed", ExpectedRevision: json.RawMessage(`"2"`), DefinitionVersion: json.RawMessage(`"1"`), IdempotencyKey: "synthetic-op"}
	tx := &riskScopeTx{t: t, raws: []string{`[{"link_id":3,"revision":1}]`, `[{"link_id":3,"revision":2}]`}}
	_, err := s.lockRiskTicketActionScopeTx(context.Background(), tx, req, 22, func(context.Context, pgx.Tx, int64, int64, []repository.RiskTicketLinkBinding) error { return nil })
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	sentinel := errors.New("synthetic risk scope denial")
	tx = &riskScopeTx{t: t, raws: []string{`[{"link_id":3,"revision":1}]`}}
	_, err = s.lockRiskTicketActionScopeTx(context.Background(), tx, req, 22, func(context.Context, pgx.Tx, int64, int64, []repository.RiskTicketLinkBinding) error { return sentinel })
	require.ErrorIs(t, err, sentinel)
	require.Equal(t, []string{"binding"}, tx.steps)
	for _, legacy := range []*model.TransitionTicketRequest{nil, {ID: 41, To: "closed"}} {
		_, err = s.lockRiskTicketActionScopeTx(context.Background(), tx, legacy, 22, func(context.Context, pgx.Tx, int64, int64, []repository.RiskTicketLinkBinding) error { return nil })
		require.ErrorIs(t, err, errcode.ErrInvalidParams)
	}
}

func TestOrdinaryCreateStillRejectsAssignedToBeforeAnyAuthorization(t *testing.T) {
	owner := int64(22)
	_, err := (&Service{}).Create(context.Background(), &model.CreateTicketRequest{AssignedTo: &owner}, 11)
	require.Error(t, err)
	require.Contains(t, err.Error(), "assigned_to")
}

package ticket

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type riskReassignmentTx struct {
	*riskRolesTx
	targetStatus int
	targetErr    error
	failAt       string
	writes       []string
	actor        int64
	from, to     string
}

func (tx *riskReassignmentTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	require.Contains(tx.t, q, "UPDATE tickets SET assigned_to")
	require.Equal(tx.t, int64(41), args[0])
	require.Equal(tx.t, int64(33), *args[1].(*int64))
	tx.writes = append(tx.writes, "assignee")
	if tx.failAt == "assignee" {
		return pgconn.CommandTag{}, errors.New("synthetic assignee failure")
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (tx *riskReassignmentTx) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	switch {
	case strings.Contains(q, "FROM users"):
		require.Equal(tx.t, int64(33), args[0])
		require.Contains(tx.t, q, "deleted_at IS NULL")
		return bridgeRowFunc(func(d ...any) error { *d[0].(*int) = tx.targetStatus; return tx.targetErr })
	case strings.Contains(q, "INSERT INTO ticket_events"):
		tx.writes = append(tx.writes, "history")
		require.Equal(tx.t, EventAssigned, args[2])
		tx.actor = args[1].(int64)
		tx.from = args[3].(string)
		tx.to = args[4].(string)
		return bridgeRowFunc(func(d ...any) error {
			if tx.failAt == "history" {
				return errors.New("synthetic history failure")
			}
			*d[0].(*int64) = 91
			*d[1].(*time.Time) = time.Unix(200, 0)
			return nil
		})
	case strings.Contains(q, "SELECT revision, updated_at, status_changed_at"):
		tx.writes = append(tx.writes, "refresh")
		return bridgeRowFunc(func(d ...any) error {
			if tx.failAt == "refresh" {
				return errors.New("synthetic refresh failure")
			}
			*d[0].(*int64) = 8
			*d[1].(*time.Time) = time.Unix(200, 0)
			*d[2].(**time.Time) = tx.ticket.StatusChangedAt
			return nil
		})
	default:
		return tx.riskRolesTx.QueryRow(ctx, q, args...)
	}
}
func riskReassignmentFixture(t *testing.T) (*Service, *riskReassignmentTx, *RiskTicketReassignmentRequest) {
	t.Helper()
	s, base, _ := riskRolesFixture(t)
	u := int64(22)
	base.ticket.AssignedTo = &u
	due, entered := time.Unix(500, 0), time.Unix(100, 0)
	base.ticket.SLADueAt = &due
	base.ticket.StatusChangedAt = &entered
	return s, &riskReassignmentTx{riskRolesTx: base, targetStatus: 1}, &RiskTicketReassignmentRequest{TicketID: 41, AssignedToUserID: 33, ExpectedRevision: 5, DefinitionVersion: 3}
}
func allowRiskReassignment(context.Context, pgx.Tx, int64, int64, int64, []repository.RiskTicketLinkBinding) error {
	return nil
}
func TestRiskReassignmentMToPKeepsUAndHandsOffInsideTransaction(t *testing.T) {
	s, tx, req := riskReassignmentFixture(t)
	ctx := riskRoleContext(11)
	scope, err := s.lockRiskTicketReassignmentScopeTx(ctx, tx, req, 11, func(_ context.Context, got pgx.Tx, id, m, p int64, b []repository.RiskTicketLinkBinding) error {
		require.Same(t, tx, got)
		require.Equal(t, int64(41), id)
		require.Equal(t, int64(11), m)
		require.Equal(t, int64(33), p)
		require.Empty(t, tx.steps)
		req.AssignedToUserID = 99
		req.ExpectedRevision = 99
		b[0].OwnerUserID = 99
		return nil
	})
	require.NoError(t, err)
	out, err := s.applyRiskTicketReassignmentTx(ctx, tx, scope, func(_ context.Context, got pgx.Tx, before, after model.Ticket, actor int64) error {
		require.Same(t, tx, got)
		require.Equal(t, []string{"assignee", "history", "refresh"}, tx.writes)
		require.Equal(t, int64(11), actor)
		require.Equal(t, int64(22), before.CreatedBy)
		require.Equal(t, int64(22), *before.AssignedTo)
		require.Equal(t, int64(22), after.CreatedBy)
		require.Equal(t, int64(33), *after.AssignedTo)
		require.Equal(t, int64(8), after.Revision)
		require.Equal(t, StatusAssigned, after.Status)
		require.Equal(t, *before.SLADueAt, *after.SLADueAt)
		require.Equal(t, *before.StatusChangedAt, *after.StatusChangedAt)
		tx.writes = append(tx.writes, "reminder-handoff")
		*before.AssignedTo = 90
		*after.AssignedTo = 91
		*after.DefinitionVersion = 99
		after.CustomData[0] = '['
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"assignee", "history", "refresh", "reminder-handoff"}, tx.writes)
	require.Equal(t, int64(11), tx.actor)
	require.Equal(t, "22", tx.from)
	require.Equal(t, "33", tx.to)
	require.Equal(t, int64(22), out.CreatedBy)
	require.Equal(t, int64(33), *out.AssignedTo)
	require.Equal(t, int64(3), *out.DefinitionVersion)
	require.Equal(t, int64(22), tx.ticket.CreatedBy)
	require.Equal(t, int64(22), *tx.ticket.AssignedTo)
	require.Equal(t, int64(22), scope.binding[0].OwnerUserID)
	require.Equal(t, personalRiskBinding, tx.binding)
}
func TestRiskReassignmentDenialsNeverWrite(t *testing.T) {
	for _, name := range []string{"risk denied", "ticket denied", "changed binding", "stale revision", "wrong definition", "missing target", "disabled target", "target lookup failure", "closed", "wrong state", "same handler", "no handoff"} {
		t.Run(name, func(t *testing.T) {
			s, tx, req := riskReassignmentFixture(t)
			ctx := riskRoleContext(11)
			locker := RiskTicketReassignmentScopeLocker(allowRiskReassignment)
			if name == "risk denied" {
				locker = func(context.Context, pgx.Tx, int64, int64, int64, []repository.RiskTicketLinkBinding) error {
					return errcode.ErrNoPermission
				}
			}
			scope, err := s.lockRiskTicketReassignmentScopeTx(ctx, tx, req, 11, locker)
			if name == "risk denied" {
				require.ErrorIs(t, err, errcode.ErrNoPermission)
				require.Empty(t, tx.writes)
				require.Empty(t, tx.steps)
				return
			}
			require.NoError(t, err)
			callback := RiskTicketReassignmentHandoff(func(context.Context, pgx.Tx, model.Ticket, model.Ticket, int64) error {
				t.Fatal("invalid command must not hand off")
				return nil
			})
			switch name {
			case "ticket denied":
				s.registry = &riskActionRegistry{}
			case "changed binding":
				tx.binding = strings.ReplaceAll(personalRiskBinding, `"revision":2`, `"revision":3`)
			case "stale revision":
				tx.ticket.Revision = 6
			case "wrong definition":
				v := int64(4)
				tx.ticket.DefinitionVersion = &v
			case "missing target":
				tx.targetErr = pgx.ErrNoRows
			case "disabled target":
				tx.targetStatus = 0
			case "target lookup failure":
				tx.targetErr = errors.New("synthetic target lookup failure")
			case "closed":
				tx.ticket.Status = StatusClosed
			case "wrong state":
				tx.ticket.Status = StatusPendingVerify
			case "same handler":
				p := int64(33)
				tx.ticket.AssignedTo = &p
			case "no handoff":
				callback = nil
			}
			_, err = s.applyRiskTicketReassignmentTx(ctx, tx, scope, callback)
			require.Error(t, err)
			require.Empty(t, tx.writes)
		})
	}
}
func TestRiskReassignmentPropagatesEveryWriteAndHandoffFailure(t *testing.T) {
	for _, name := range []string{"assignee", "history", "refresh", "handoff"} {
		t.Run(name, func(t *testing.T) {
			s, tx, req := riskReassignmentFixture(t)
			tx.failAt = name
			ctx := riskRoleContext(11)
			scope, err := s.lockRiskTicketReassignmentScopeTx(ctx, tx, req, 11, allowRiskReassignment)
			require.NoError(t, err)
			sentinel := errors.New("synthetic handoff failed")
			out, err := s.applyRiskTicketReassignmentTx(ctx, tx, scope, func(context.Context, pgx.Tx, model.Ticket, model.Ticket, int64) error {
				require.Equal(t, "handoff", name)
				return sentinel
			})
			require.Error(t, err)
			require.Nil(t, out)
			if name == "handoff" {
				require.ErrorIs(t, err, sentinel)
			}
		})
	}
}
func TestRiskReassignmentRejectsBareGuardAndInvalidRequest(t *testing.T) {
	s, tx, req := riskReassignmentFixture(t)
	_, err := s.LockRiskTicketReassignmentScopeTx(context.Background(), tx, req, 11, allowRiskReassignment)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	_, err = s.ApplyRiskTicketReassignmentTx(context.Background(), tx, nil, nil)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	for _, name := range []string{"nil", "ticket", "target", "revision", "definition", "actor", "locker"} {
		t.Run(name, func(t *testing.T) {
			r := *req
			actor := int64(11)
			locker := RiskTicketReassignmentScopeLocker(allowRiskReassignment)
			rp := &r
			switch name {
			case "nil":
				rp = nil
			case "ticket":
				r.TicketID = 0
			case "target":
				r.AssignedToUserID = 0
			case "revision":
				r.ExpectedRevision = 0
			case "definition":
				r.DefinitionVersion = 0
			case "actor":
				actor = 0
			case "locker":
				locker = nil
			}
			_, err := s.lockRiskTicketReassignmentScopeTx(context.Background(), tx, rp, actor, locker)
			require.ErrorIs(t, err, errcode.ErrInvalidParams)
		})
	}
}

func TestRiskReassignmentScopeCannotCrossServiceTransactionOrBeReused(t *testing.T) {
	s, tx, req := riskReassignmentFixture(t)
	ctx := riskRoleContext(11)
	scope, err := s.lockRiskTicketReassignmentScopeTx(ctx, tx, req, 11, allowRiskReassignment)
	require.NoError(t, err)
	handoff := RiskTicketReassignmentHandoff(func(context.Context, pgx.Tx, model.Ticket, model.Ticket, int64) error { return nil })
	other := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	_, err = other.applyRiskTicketReassignmentTx(ctx, tx, scope, handoff)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	_, otherTx, _ := riskReassignmentFixture(t)
	_, err = s.applyRiskTicketReassignmentTx(ctx, otherTx, scope, handoff)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.Empty(t, tx.writes)
	require.Empty(t, otherTx.writes)
	_, err = s.applyRiskTicketReassignmentTx(ctx, tx, scope, handoff)
	require.NoError(t, err)
	writes := append([]string(nil), tx.writes...)
	_, err = s.applyRiskTicketReassignmentTx(ctx, tx, scope, handoff)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.Equal(t, writes, tx.writes)
}
func TestRiskReassignmentScopeRejectsBindingChangeBeforeTicketLock(t *testing.T) {
	s, tx, req := riskReassignmentFixture(t)
	_, err := s.lockRiskTicketReassignmentScopeTx(riskRoleContext(11), tx, req, 11, func(context.Context, pgx.Tx, int64, int64, int64, []repository.RiskTicketLinkBinding) error {
		tx.binding = strings.ReplaceAll(personalRiskBinding, `"revision":2`, `"revision":3`)
		return nil
	})
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	require.Empty(t, tx.steps)
	require.Empty(t, tx.writes)
}

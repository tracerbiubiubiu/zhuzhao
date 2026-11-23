package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type initialRiskTx struct {
	pgx.Tx
	t            *testing.T
	steps        []string
	actor, owner int64
	hours        *int
	failAt       string
}

func (tx *initialRiskTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	step := "status"
	if strings.Contains(q, "SET assigned_to") {
		step = "assignee"
		tx.owner = *args[1].(*int64)
	}
	tx.steps = append(tx.steps, step)
	if tx.failAt == step {
		return pgconn.CommandTag{}, errors.New("synthetic " + step)
	}
	return pgconn.NewCommandTag("UPDATE 1"), nil
}
func (tx *initialRiskTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	switch {
	case strings.Contains(q, "SET sla_due_at"):
		tx.steps = append(tx.steps, "sla")
		tx.hours = args[1].(*int)
		return bridgeRowFunc(func(d ...any) error { due := time.Unix(3600, 0); *d[0].(**time.Time) = &due; return nil })
	case strings.Contains(q, "INSERT INTO ticket_events"):
		tx.steps = append(tx.steps, "assigned-event")
		tx.actor = args[1].(int64)
		require.Equal(tx.t, EventAssigned, args[2])
		require.Equal(tx.t, "", args[3])
		require.Equal(tx.t, "22", args[4])
		return bridgeRowFunc(func(d ...any) error { *d[0].(*int64) = 201; *d[1].(*time.Time) = time.Unix(1, 0); return nil })
	default:
		tx.t.Fatalf("unexpected initial assignment query %s", q)
		return nil
	}
}

type initialRiskDelegation struct{}

func (initialRiskDelegation) IsOrgAdminOrOwner(context.Context, int64, int64) (bool, error) {
	return false, nil
}
func (initialRiskDelegation) IsAncestorOwner(context.Context, int64, int64, string) (bool, error) {
	return false, nil
}
func (initialRiskDelegation) HasOrgManagePermission(context.Context, int64) (bool, error) {
	return false, nil
}

func TestInitialRiskAssignmentPreservesActorAndSLA(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	p := syntheticPreparedCreate()
	p.ttype.Transitions = json.RawMessage(`{"open":["assigned"]}`)
	p.ttype.TransitionMeta = json.RawMessage(`{"assigned":{"sla_due_hours":2}}`)
	ticket := p.ticket
	ticket.ID = 101
	tx := &initialRiskTx{t: t}
	require.NoError(t, s.assignInitialRiskTicketTx(context.Background(), tx, p, &ticket, 22))
	require.Equal(t, []string{"assignee", "status", "sla", "assigned-event"}, tx.steps)
	require.Equal(t, int64(22), tx.owner)
	require.Equal(t, int64(11), tx.actor)
	require.Equal(t, int64(22), ticket.CreatedBy)
	require.Equal(t, int64(22), *ticket.AssignedTo)
	require.Equal(t, StatusAssigned, ticket.Status)
	require.NotNil(t, ticket.SLADueAt)
	require.NotNil(t, tx.hours)
	require.Equal(t, 2, *tx.hours)
}

func TestInitialRiskAssignmentCannotSkipDefinitionGuards(t *testing.T) {
	for _, tc := range []struct {
		name, graph, meta string
		want              error
	}{
		{"missing edge", `{"open":["closed"]}`, `{}`, errcode.ErrTicketInvalidTransition},
		{"condition uses actual dispatcher", `{"open":["assigned"]}`, `{"open>assigned":{"roles":["creator"]}}`, errcode.ErrNoPermission},
		{"validator", `{"open":["assigned"]}`, `{"open>assigned":{"validator":"priority == 1"}}`, nil},
		{"evidence edge", `{"open":["assigned"]}`, `{"open>assigned":{"comment_required":true}}`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{ticketRepo: repository.NewTicketRepo(nil), delegation: initialRiskDelegation{}}
			p := syntheticPreparedCreate()
			p.ttype.Transitions = []byte(tc.graph)
			p.ttype.TransitionMeta = []byte(tc.meta)
			ticket := p.ticket
			ticket.ID = 101
			tx := &initialRiskTx{t: t}
			ctx := context.WithValue(context.Background(), ticketCommandRolesKey{}, ticketCommandRoles{actor: 11})
			err := s.assignInitialRiskTicketTx(ctx, tx, p, &ticket, 22)
			if tc.want != nil {
				require.ErrorIs(t, err, tc.want)
			} else {
				require.Error(t, err)
			}
			require.Empty(t, tx.steps)
			require.Equal(t, StatusOpen, ticket.Status)
			require.Nil(t, ticket.AssignedTo)
		})
	}
}

func TestInitialRiskAssignmentRejectsWrongSnapshotAndPropagatesWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name     string
		status   string
		assignee *int64
		fail     string
	}{
		{name: "wrong state", status: StatusClosed}, {name: "already assigned", status: StatusOpen, assignee: new(int64)},
		{name: "assignee failure", status: StatusOpen, fail: "assignee"}, {name: "status failure", status: StatusOpen, fail: "status"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
			p := syntheticPreparedCreate()
			p.ttype.Transitions = []byte(`{"open":["assigned"]}`)
			ticket := p.ticket
			ticket.Status = tc.status
			ticket.AssignedTo = tc.assignee
			tx := &initialRiskTx{t: t, failAt: tc.fail}
			require.Error(t, s.assignInitialRiskTicketTx(context.Background(), tx, p, &ticket, 22))
			require.NotContains(t, tx.steps, "assigned-event")
		})
	}
}

type riskDispatchKernelTx struct {
	bridge  *bridgeRecordingTx
	initial *initialRiskTx
}

func (tx *riskDispatchKernelTx) Exec(ctx context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	if strings.Contains(q, "SET assigned_to") || strings.Contains(q, "SET status =") {
		tag, err := tx.initial.Exec(ctx, q, args...)
		tx.bridge.steps = append(tx.bridge.steps, tx.initial.steps[len(tx.initial.steps)-1])
		return tag, err
	}
	return tx.bridge.Exec(ctx, q, args...)
}
func (tx *riskDispatchKernelTx) QueryRow(ctx context.Context, q string, args ...any) pgx.Row {
	if strings.Contains(q, "SELECT revision, updated_at, status_changed_at") {
		tx.bridge.steps = append(tx.bridge.steps, "refresh")
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*int64) = 3
			*d[1].(*time.Time) = time.Unix(2, 0)
			at := time.Unix(2, 0)
			*d[2].(**time.Time) = &at
			return nil
		})
	}
	if strings.Contains(q, "INSERT INTO ticket_events") && args[2] == EventAssigned || strings.Contains(q, "SET sla_due_at") && tx.initial.owner != 0 {
		row := tx.initial.QueryRow(ctx, q, args...)
		tx.bridge.steps = append(tx.bridge.steps, tx.initial.steps[len(tx.initial.steps)-1])
		return row
	}
	return tx.bridge.QueryRow(ctx, q, args...)
}
func (tx *riskDispatchKernelTx) Begin(context.Context) (pgx.Tx, error) { panic("must reuse outer tx") }
func (tx *riskDispatchKernelTx) Commit(context.Context) error          { panic("must not commit outer tx") }
func (tx *riskDispatchKernelTx) Rollback(context.Context) error        { panic("must not rollback outer tx") }
func (tx *riskDispatchKernelTx) CopyFrom(context.Context, pgx.Identifier, []string, pgx.CopyFromSource) (int64, error) {
	panic("unused")
}
func (tx *riskDispatchKernelTx) SendBatch(context.Context, *pgx.Batch) pgx.BatchResults {
	panic("unused")
}
func (tx *riskDispatchKernelTx) LargeObjects() pgx.LargeObjects { panic("unused") }
func (tx *riskDispatchKernelTx) Prepare(context.Context, string, string) (*pgconn.StatementDescription, error) {
	panic("unused")
}
func (tx *riskDispatchKernelTx) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unused")
}
func (tx *riskDispatchKernelTx) Conn() *pgx.Conn { panic("unused") }

func TestRiskDispatchWriteAssignsBeforeLinkAndFilesAndRefreshesRevision(t *testing.T) {
	s := &Service{ticketRepo: repository.NewTicketRepo(nil)}
	p := syntheticPreparedCreate()
	p.ttype.Transitions = []byte(`{"open":["assigned"]}`)
	ticket := p.ticket
	tx := &riskDispatchKernelTx{bridge: &bridgeRecordingTx{t: t}, initial: &initialRiskTx{t: t}}
	err := s.writeInitialRiskDispatchTx(context.Background(), tx, p, &ticket, 22, func(_ context.Context, supplied pgx.Tx, id int64) error {
		require.Same(t, tx, supplied)
		require.Equal(t, int64(101), id)
		require.Equal(t, "TK-synthetic-1", ticket.HumanKey)
		require.Equal(t, StatusAssigned, ticket.Status)
		require.Equal(t, int64(22), *ticket.AssignedTo)
		tx.bridge.steps = append(tx.bridge.steps, "link")
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"ticket", "sla", "created", "key", "assignee", "status", "sla", "assigned-event", "link", "refresh"}, tx.bridge.steps)
	require.Equal(t, int64(22), tx.bridge.creator)
	require.Equal(t, int64(11), tx.bridge.actor)
	require.Equal(t, int64(11), tx.initial.actor)
	require.Equal(t, int64(3), ticket.Revision)
	require.Equal(t, time.Unix(2, 0), ticket.UpdatedAt)
	require.Equal(t, time.Unix(2, 0), *ticket.StatusChangedAt)
}

func TestRiskDispatchNotificationSnapshotOwnsAssignee(t *testing.T) {
	p := syntheticPreparedCreate()
	owner := int64(22)
	p.ticket.AssignedTo = &owner
	snapshot := riskDispatchNotificationSnapshot(&p.ticket)
	owner = 99
	p.ticket.Title = "changed"
	require.Equal(t, int64(22), *snapshot.AssignedTo)
	require.Equal(t, "", snapshot.Title)
}

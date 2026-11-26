package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

// This fake models the owning transaction only. Unexpected reads/writes fail so
// a guard that moves behind file binding or a business write cannot pass silently.
type riskEvidenceTx struct {
	pgx.Tx
	t            *testing.T
	revision     int64
	steps        []string
	data         json.RawMessage
	eventActors  []int64
	eventActions []string
	receiptActor int64
	receiptKey   string
	receiptRows  int64
	dueHours     *int
}

func (tx *riskEvidenceTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	switch {
	case strings.Contains(q, "SELECT revision FROM tickets"):
		tx.steps = append(tx.steps, "revision")
		return bridgeRowFunc(func(d ...any) error { *d[0].(*int64) = tx.revision; return nil })
	case strings.Contains(q, "SET custom_data="):
		tx.steps = append(tx.steps, "ticket-write")
		require.Equal(tx.t, []any{int64(41), int64(5), int64(3), StatusAssigned, StatusPendingVerify}, args[:5])
		tx.data = append(json.RawMessage(nil), args[5].(json.RawMessage)...)
		return bridgeRowFunc(func(d ...any) error {
			*d[0].(*string) = StatusPendingVerify
			*d[1].(*int64) = 6
			*d[2].(*int64) = 3
			at := time.Unix(100, 0)
			*d[3].(**time.Time) = &at
			return nil
		})
	case strings.Contains(q, "SET sla_due_at"):
		tx.steps = append(tx.steps, "sla")
		tx.dueHours = args[1].(*int)
		return bridgeRowFunc(func(d ...any) error {
			due := time.Unix(100, 0).Add(4 * time.Hour)
			*d[0].(**time.Time) = &due
			return nil
		})
	case strings.Contains(q, "INSERT INTO ticket_events"):
		tx.steps = append(tx.steps, "event")
		tx.eventActors = append(tx.eventActors, args[1].(int64))
		tx.eventActions = append(tx.eventActions, args[2].(string))
		return bridgeRowFunc(func(d ...any) error { *d[0].(*int64) = 70; *d[1].(*time.Time) = time.Unix(100, 0); return nil })
	case strings.Contains(q, "INSERT INTO ticket_comments"):
		tx.steps = append(tx.steps, "comment")
		require.Equal(tx.t, int64(22), args[1])
		require.Equal(tx.t, "synthetic conclusion", args[2])
		return bridgeRowFunc(func(d ...any) error { *d[0].(*int64) = 71; *d[1].(*time.Time) = time.Unix(100, 0); return nil })
	default:
		tx.t.Fatalf("unexpected evidence transaction query: %s", q)
		return nil
	}
}
func (tx *riskEvidenceTx) Exec(_ context.Context, q string, args ...any) (pgconn.CommandTag, error) {
	require.Contains(tx.t, q, "INSERT INTO ticket_evidence_transition_receipts")
	tx.steps = append(tx.steps, "receipt")
	tx.receiptActor = args[1].(int64)
	tx.receiptKey = args[2].(string)
	return pgconn.NewCommandTag(fmt.Sprintf("INSERT 0 %d", tx.receiptRows)), nil
}
func riskEvidenceFixture(t *testing.T) (*Service, *model.Ticket, *model.TicketDefinitionSnapshot, evidenceTransitionRequest) {
	t.Helper()
	version := int64(3)
	before := &model.Ticket{ID: 41, TypeCode: "incident", Status: StatusAssigned, DefinitionVersion: &version, Revision: 5, CustomData: json.RawMessage(`{"result":"earlier","unknown":"retained"}`)}
	snapshot := &model.TicketDefinitionSnapshot{Type: model.TicketType{Code: "incident", Transitions: json.RawMessage(`{"assigned":["pending_verify"]}`), TransitionMeta: json.RawMessage(`{"assigned>pending_verify":{"collect_fields":[{"key":"result","required":true}],"validator":"custom_data[\"result\"] == \"accepted\"","comment_required":true},"pending_verify":{"sla_due_hours":4}}`)}, Fields: []*model.TicketTypeField{{FieldKey: "result", FieldType: "input"}}}
	cmd, err := parseEvidenceTransitionRequest(&model.TransitionTicketRequest{ID: 41, To: StatusPendingVerify, Comment: "synthetic conclusion", ExpectedRevision: json.RawMessage(`"5"`), DefinitionVersion: json.RawMessage(`"3"`), CustomDataPatch: json.RawMessage(`{"result":"accepted"}`), IdempotencyKey: "synthetic-risk-action"})
	require.NoError(t, err)
	return &Service{ticketRepo: repository.NewTicketRepo(nil)}, before, snapshot, cmd
}
func TestRiskEvidenceGuardRejectsBeforeFileBindingOrTicketWrite(t *testing.T) {
	svc, before, snapshot, cmd := riskEvidenceFixture(t)
	snapshot.Fields = append(snapshot.Fields, &model.TicketTypeField{FieldKey: "evidence", FieldType: "file"})
	snapshot.Type.TransitionMeta = json.RawMessage(`{"assigned>pending_verify":{"collect_fields":[{"key":"result","required":true},{"key":"evidence","required":true}],"validator":"custom_data[\"result\"] == \"accepted\""}}`)
	cmd.patch = json.RawMessage(`{"result":"accepted","evidence":["synthetic-object"]}`)
	tx := &riskEvidenceTx{t: t, revision: 5}
	denied := errors.New("synthetic linked-risk guard denial")
	called := false
	_, err := svc.applyEvidenceTransitionGuardedTx(context.Background(), tx, cmd, before, snapshot, 22, func(candidate *model.Ticket) error {
		called = true
		require.JSONEq(t, `{"result":"accepted","evidence":["synthetic-object"],"unknown":"retained"}`, string(candidate.CustomData))
		require.Equal(t, []string{"revision"}, tx.steps)
		return denied
	})
	require.ErrorIs(t, err, denied)
	require.True(t, called)
	require.Equal(t, []string{"revision"}, tx.steps, "guard failure must stop before file binding, ticket mutation, history, SLA and receipt")
	require.JSONEq(t, `{"result":"earlier","unknown":"retained"}`, string(before.CustomData))
}
func TestRiskEvidenceInvalidCandidateNeverInvokesGuard(t *testing.T) {
	for _, name := range []string{"stale revision", "wrong definition", "illegal graph", "validator denial", "missing comment", "missing evidence"} {
		t.Run(name, func(t *testing.T) {
			svc, before, snapshot, cmd := riskEvidenceFixture(t)
			tx := &riskEvidenceTx{t: t, revision: 5}
			switch name {
			case "stale revision":
				tx.revision = 6
			case "wrong definition":
				cmd.version = 4
			case "illegal graph":
				snapshot.Type.Transitions = json.RawMessage(`{"assigned":["open"]}`)
			case "validator denial":
				cmd.patch = json.RawMessage(`{"result":"rejected"}`)
			case "missing comment":
				cmd.comment = ""
			case "missing evidence":
				cmd.patch = json.RawMessage(`{}`)
			}
			called := false
			_, err := svc.applyEvidenceTransitionGuardedTx(context.Background(), tx, cmd, before, snapshot, 22, func(*model.Ticket) error { called = true; return nil })
			require.Error(t, err)
			require.False(t, called)
			for _, step := range tx.steps {
				require.Equal(t, "revision", step, "invalid candidate must not write")
			}
		})
	}
}
func TestRiskEvidenceLegalActionPreservesHistorySLAAndReceipt(t *testing.T) {
	svc, before, snapshot, cmd := riskEvidenceFixture(t)
	tx := &riskEvidenceTx{t: t, revision: 5, receiptRows: 1}
	result, err := svc.applyEvidenceTransitionGuardedTx(context.Background(), tx, cmd, before, snapshot, 22, func(candidate *model.Ticket) error {
		tx.steps = append(tx.steps, "risk-guard")
		require.JSONEq(t, `{"result":"accepted","unknown":"retained"}`, string(candidate.CustomData))
		candidate.CustomData[0] = '[' // Guard sees its own byte slice, never the payload written below.
		return nil
	})
	require.NoError(t, err)
	require.Equal(t, []string{"revision", "risk-guard", "ticket-write", "sla", "event", "comment", "event", "receipt"}, tx.steps)
	require.JSONEq(t, `{"result":"accepted","unknown":"retained"}`, string(tx.data))
	require.Equal(t, []int64{22, 22}, tx.eventActors)
	require.Equal(t, []string{EventStatusChanged, EventComment}, tx.eventActions)
	require.Equal(t, 4, *tx.dueHours)
	require.Equal(t, int64(22), tx.receiptActor)
	require.Equal(t, cmd.key, tx.receiptKey)
	require.Equal(t, StatusPendingVerify, result.Status)
	require.Equal(t, int64(6), result.Revision)
	require.Equal(t, int64(3), result.DefinitionVersion)
	require.Equal(t, StatusAssigned, before.Status)
	require.JSONEq(t, `{"result":"earlier","unknown":"retained"}`, string(before.CustomData))
}
func TestRiskEvidenceReceiptCollisionPropagatesToOwningTransaction(t *testing.T) {
	svc, before, snapshot, cmd := riskEvidenceFixture(t)
	tx := &riskEvidenceTx{t: t, revision: 5, receiptRows: 0}
	_, err := svc.applyEvidenceTransitionGuardedTx(context.Background(), tx, cmd, before, snapshot, 22, func(*model.Ticket) error { return nil })
	require.ErrorIs(t, err, errcode.ErrConflict)
	require.Equal(t, "receipt", tx.steps[len(tx.steps)-1])
}

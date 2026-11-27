//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3al"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type b3TaskPeer struct {
	t           *testing.T
	runs        map[string]json.RawMessage
	submissions int
	retried     []string
}

func (*b3TaskPeer) CallerAK() string { return "synthetic-core" }

func (p *b3TaskPeer) GetTask(_ context.Context, taskID string) (json.RawMessage, error) {
	if result, ok := p.runs[taskID]; ok {
		return result, nil
	}
	return nil, errcode.New(errcode.ErrNotFound.Code, "Taskrunner reported missing task")
}

func (p *b3TaskPeer) Submit(_ context.Context, input taskrunner.SubmitRequest) (*taskrunner.SubmitResponse, error) {
	p.submissions++
	var err error
	p.runs[input.TaskID], err = json.Marshal(map[string]any{
		"task_id": input.TaskID, "action": input.Action,
		"submitted_by": "service:" + p.CallerAK(), "params": string(input.Params),
	})
	require.NoError(p.t, err)
	return nil, fmt.Errorf("synthetic lost response after Taskrunner accepted")
}

func (p *b3TaskPeer) RetryTask(_ context.Context, taskID, _ string) (json.RawMessage, error) {
	p.retried = append(p.retried, taskID)
	return nil, nil
}

type b3ReadPeer struct{ calls int }

func (p *b3ReadPeer) Get(_ context.Context, command b3al.Command) (b3al.ReadResult, error) {
	p.calls++
	return b3al.ReadResult{
		TypeName: command.TypeName, DocumentID: command.DocumentID, Generation: command.Generation,
		DocumentRevision: "99", SourceEventID: command.SourceEventID, SourceEventRevision: command.SourceEventRevision,
		Values: map[string]any{"name": "当前值"},
	}, nil
}

func (*b3ReadPeer) Update(context.Context, b3al.Command) (b3al.UpdateResult, error) {
	return b3al.UpdateResult{}, fmt.Errorf("unexpected write from a read-only operation")
}

func (*b3ReadPeer) LookupUpdate(context.Context, b3al.Command) (b3al.UpdateResult, error) {
	return b3al.UpdateResult{}, fmt.Errorf("unexpected write receipt from a read-only operation")
}

func TestB3SourceCommitAndRecoverByOriginalIdentity(t *testing.T) {
	ctx := context.Background()
	generation := strconv.FormatUint(rand.Uint64N(1_000_000_000)+1, 10)
	cfg := service.B3Subscription{
		TypeName: "b3_fixture", Generation: generation, ConsumerID: "b3-test-consumer-0123",
		RuleID: "b3-test-rule-0123456789", RuleVersion: "1", GrantRef: "b3-test-grant-" + uniqueSuffix(),
		GrantRevision: "1", SubjectID: "b3-test-service", Fields: []string{"name"},
	}
	_, err := testPool.Exec(ctx, `
		INSERT INTO b3_grants(grant_ref, revision, subject_type, subject_id, type_name, generation, read_fields, enabled)
		VALUES ($1,1,'service',$2,$3,$4,$5,true)
	`, cfg.GrantRef, cfg.SubjectID, cfg.TypeName, generation, cfg.Fields)
	require.NoError(t, err)
	store := repository.NewB3Store(testPool)
	tr := &b3TaskPeer{t: t, runs: make(map[string]json.RawMessage)}
	al := &b3ReadPeer{}
	newFlow := func() *service.B3Flow { return service.NewB3Flow(testPool, store, tr, al, nil, "", cfg) }
	flow := newFlow()
	event := fmt.Sprintf(`{"specversion":"1.0","datacontenttype":"application/json",
		"id":"al-test-event-%s","source":"urn:zhuzhao:activelist:b3_fixture","type":"al.document.created.v1",
		"time":"2026-11-26T00:00:00Z","subject":"b3_fixture/42","data":{"stream_id":"activelist-stream-b3_fixture-%s",
		"generation":"%s","cursor":"1","document_id":"42","document_revision":"7","changed_fields":["name"],
		"event_values":{"name":"事件值"},"operation_id":"al-test-op-123456789","causality":{"root_id":"al-test-op-123456789",
		"root_budget_ref":"al-test-budget-12345","depth":0,"rule_path":[]}}}`, generation, generation, generation)
	packet := b3protocol.SourceDelivery{SchemaVersion: "1", DeliveryID: "al-test-delivery-12345", ConsumerID: cfg.ConsumerID, Event: json.RawMessage(event)}
	ack, err := flow.Push(ctx, packet)
	require.NoError(t, err)
	require.Equal(t, "durably_staged", ack.State)
	hasHistory, err := store.HasHistory(ctx)
	require.NoError(t, err)
	require.True(t, hasHistory, "paused B3 must still protect its historical task reads")

	// Restart the receiver and replay the original source event with a new transport delivery.
	flow = newFlow()
	packet.DeliveryID = "al-test-delivery-abcde"
	duplicate, err := flow.Push(ctx, packet)
	require.NoError(t, err)
	require.Equal(t, ack.StageCommitRef, duplicate.StageCommitRef)
	require.Equal(t, packet.DeliveryID, duplicate.DeliveryID)
	require.Equal(t, ack.PayloadSHA256, duplicate.PayloadSHA256)

	require.NoError(t, flow.Handle(ctx, nil))
	require.Equal(t, 1, tr.submissions)
	var operation, task string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT operation_id,task_id FROM command_receipts WHERE payload->>'event_id'=$1`, ack.EventID).Scan(&operation, &task))
	_, err = testPool.Exec(ctx, "UPDATE business_intent_outbox SET next_attempt_at=now() WHERE operation_id=$1", operation)
	require.NoError(t, err)
	require.NoError(t, newFlow().Handle(ctx, nil))
	require.Equal(t, 1, tr.submissions, "a lost reply must recover the original Taskrunner record")
	// A previously accepted run may use up its delivery retries during a
	// Core outage. Reconcile the original Taskrunner id, not a second task.
	var dead map[string]any
	require.NoError(t, json.Unmarshal(tr.runs[task], &dead))
	dead["status"] = "dead"
	tr.runs[task], err = json.Marshal(dead)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, "UPDATE business_intent_outbox SET next_attempt_at=now() WHERE operation_id=$1", operation)
	require.NoError(t, err)
	require.NoError(t, newFlow().Handle(ctx, nil))
	require.Equal(t, []string{task}, tr.retried)
	require.Equal(t, 1, tr.submissions)

	expired, err := store.Execution(ctx, task)
	require.NoError(t, err)
	require.NoError(t, store.ClaimExecution(ctx, &expired))
	_, err = testPool.Exec(ctx, "UPDATE command_receipts SET execution_until=now()-interval '1 second' WHERE task_id=$1", task)
	require.NoError(t, err)
	applied, err := store.Complete(ctx, expired, "succeeded", "", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.False(t, applied, "an expired execution must not finish even without a takeover")

	params, err := json.Marshal(b3protocol.TaskParams{OperationID: operation})
	require.NoError(t, err)
	callback := service.B3Callback{ProtocolVersion: 2, ActionVersion: 1, TaskID: task, Action: b3protocol.Read,
		OperationID: operation, DeliveryID: "tr-attempt-123456789", Params: params}
	first, err := flow.Callback(ctx, callback)
	require.NoError(t, err)
	callback.DeliveryID = "tr-attempt-987654321"
	same, err := newFlow().Callback(ctx, callback)
	require.NoError(t, err)
	require.Equal(t, first, same)
	require.Equal(t, 1, al.calls)
	status, err := flow.Status(ctx, task, operation)
	require.NoError(t, err)
	require.Contains(t, string(status.Result), "当前值")
	_, err = testPool.Exec(ctx, "UPDATE b3_grants SET enabled=false WHERE grant_ref=$1", cfg.GrantRef)
	require.NoError(t, err)
	_, err = flow.Status(ctx, task, operation)
	require.ErrorIs(t, err, errcode.ErrForbidden)
}

type b3RecoveryPeer struct {
	lookups, writes  []string
	receiptAvailable bool
}

func (*b3RecoveryPeer) Get(context.Context, b3al.Command) (b3al.ReadResult, error) {
	return b3al.ReadResult{}, fmt.Errorf("unexpected document read")
}

func (p *b3RecoveryPeer) LookupUpdate(_ context.Context, cmd b3al.Command) (b3al.UpdateResult, error) {
	p.lookups = append(p.lookups, cmd.OperationID)
	if !p.receiptAvailable {
		return b3al.UpdateResult{}, &b3al.HTTPError{Status: 404}
	}
	return b3al.UpdateResult{DocumentRevision: "2"}, nil
}

func (p *b3RecoveryPeer) Update(_ context.Context, cmd b3al.Command) (b3al.UpdateResult, error) {
	p.writes = append(p.writes, cmd.OperationID)
	return b3al.UpdateResult{DocumentRevision: "2"}, nil
}

func TestB3SentUpdateRecovery(t *testing.T) {
	for _, tt := range []struct {
		name                             string
		sent, canceled, expired, receipt bool
		wantLookup, wantWrite            bool
		status, reason                   string
	}{
		{name: "missing_receipt_stays_unknown", sent: true, wantLookup: true, status: "pending", reason: "document_outcome_unconfirmed"},
		{name: "first_write_when_unsent", wantWrite: true, status: "succeeded"},
		{name: "expired_uncertain", sent: true, expired: true, wantLookup: true, status: "pending", reason: "document_outcome_unconfirmed"},
		{name: "expired_receipt", sent: true, expired: true, receipt: true, wantLookup: true, status: "succeeded"},
		{name: "canceled_uncertain", sent: true, canceled: true, wantLookup: true, status: "pending", reason: "document_outcome_unconfirmed"},
		{name: "canceled_receipt", sent: true, canceled: true, receipt: true, wantLookup: true, status: "succeeded"},
		{name: "never_sent_canceled", canceled: true, status: "failed_no_effect", reason: "task_delivery_canceled"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			id := int64(rand.Uint64N(1<<50)) + 1
			grant := fmt.Sprintf("recovery-grant-%d", id)
			op := b3protocol.Identity("op_ticket_", fmt.Sprint(id))
			task := b3protocol.TaskID(op)
			_, err := testPool.Exec(ctx, `INSERT INTO b3_grants(grant_ref,revision,subject_type,subject_id,type_name,generation,document_id,write_fields,enabled)
				VALUES($1,1,'human','42','sample',1,7,ARRAY['name'],true)`, grant)
			require.NoError(t, err)
			payload, err := json.Marshal(b3protocol.DocumentUpdate{TypeName: "sample", DocumentID: "7", Generation: "1", ExpectedRevision: "1",
				Patch: map[string]any{"name": "synthetic"}, GrantRef: grant, GrantRevision: "1"})
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO command_receipts(operation_id,task_id,action,ticket_id,ticket_revision,node,
				definition_hash,action_index,actor_id,transition_roles,payload,remote_started,created_at)
				VALUES($1,$2,'activelist.document.update',$3,1,'assigned','synthetic',0,42,'{}',$4,$5,
				now() - CASE WHEN $6 THEN interval '2 hours' ELSE interval '1 second' END)`, op, task, id, payload, tt.sent, tt.expired)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, "INSERT INTO business_intent_outbox(operation_id) VALUES($1)", op)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := testPool.Exec(ctx, "DELETE FROM business_intent_outbox WHERE operation_id=$1", op)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, "DELETE FROM command_receipts WHERE operation_id=$1", op)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, "DELETE FROM b3_grants WHERE grant_ref=$1", grant)
				require.NoError(t, err)
			})
			params, err := json.Marshal(b3protocol.TaskParams{OperationID: op})
			require.NoError(t, err)
			tr := &b3TaskPeer{t: t, runs: make(map[string]json.RawMessage)}
			tr.runs[task], err = json.Marshal(map[string]any{"task_id": task, "action": b3protocol.Update,
				"submitted_by": "service:zhuzhao-b3", "params": string(params), "status": "canceled"})
			require.NoError(t, err)
			al := &b3RecoveryPeer{receiptAvailable: tt.receipt}
			store := repository.NewB3Store(testPool)
			flow := service.NewB3Flow(testPool, store, tr, al, func(ctx context.Context, _ repository.B3Execution, _ string, execute func(context.Context) error) error {
				return execute(ctx)
			}, "", service.B3Subscription{})
			if tt.canceled {
				require.NoError(t, flow.Handle(ctx, nil))
			} else {
				_, err = flow.Callback(ctx, service.B3Callback{ProtocolVersion: 2, ActionVersion: 1, TaskID: task, Action: b3protocol.Update,
					OperationID: op, DeliveryID: "test-update-delivery-12345", Params: params})
				if tt.status == "pending" {
					require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
				} else {
					require.NoError(t, err)
				}
			}
			run, err := store.Execution(ctx, task)
			require.NoError(t, err)
			require.Equal(t, tt.status, run.Status)
			require.Equal(t, tt.reason, run.Reason)
			if tt.wantLookup {
				require.Equal(t, []string{op}, al.lookups)
			} else {
				require.Empty(t, al.lookups)
			}
			if tt.wantWrite {
				require.Equal(t, []string{op}, al.writes)
			} else {
				require.Empty(t, al.writes)
			}
			require.Empty(t, tr.retried)
			require.Zero(t, tr.submissions)
			if tt.name == "missing_receipt_stays_unknown" {
				al.receiptAvailable = true
				// The Taskrunner ledger may be unavailable even after ActiveList commits.
				delete(tr.runs, task)
				require.NoError(t, flow.Handle(ctx, nil))
				recovered, err := store.Execution(ctx, task)
				require.NoError(t, err)
				require.Equal(t, "succeeded", recovered.Status)
				require.Equal(t, []string{op, op}, al.lookups)
				require.Empty(t, al.writes)
				require.Zero(t, tr.submissions)
			}
		})
	}
}

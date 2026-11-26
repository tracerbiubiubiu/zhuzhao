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
		"submitted_by": input.SubmittedBy, "params": string(input.Params),
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

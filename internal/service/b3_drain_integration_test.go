//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestB3DrainOnlyAcknowledgesCommittedSourceIdentities(t *testing.T) {
	ctx := context.Background()
	store := repository.NewB3Store(testPool)
	cfg := service.B3Subscription{
		TypeName: "drain_fixture", Generation: "1", ConsumerID: "drain-source-012345",
		RuleID: "drain-rule-012345", RuleVersion: "1", GrantRef: "drain-grant-012345",
		GrantRevision: "1", SubjectID: "drain-reader", Fields: []string{"name"},
	}
	flow := service.NewB3Flow(testPool, store, nil, nil, nil, "", cfg)
	suffix := uniqueSuffix()
	id := "drain-event-" + suffix
	const source = "urn:zhuzhao:activelist:drain_fixture"
	packet := b3protocol.SourceDelivery{SchemaVersion: "1", DeliveryID: "drain-delivery-012345", ConsumerID: cfg.ConsumerID,
		Event: json.RawMessage(fmt.Sprintf(`{"specversion":"1.0","datacontenttype":"application/json",
		"id":%q,"source":%q,"type":"al.document.created.v1",
		"time":"2026-11-26T00:00:00Z","subject":"drain_fixture/42","data":{"stream_id":"activelist-stream-drain_fixture-1",
		"generation":"1","cursor":"1","document_id":"42","document_revision":"7","changed_fields":["name"],
		"event_values":{"name":"original"},"operation_id":"drain-op-123456789","causality":{"root_id":"drain-op-123456789",
		"root_budget_ref":"drain-budget-12345","depth":0,"rule_path":[]}}}`, id, source))}
	ack, err := flow.Push(ctx, packet)
	require.NoError(t, err)
	op := b3protocol.Identity("op_evt_", source+"/"+id+"/"+cfg.RuleID+"/"+cfg.RuleVersion+"/0")
	t.Cleanup(func() {
		_, err := testPool.Exec(ctx, "DELETE FROM business_intent_outbox WHERE operation_id=$1", op)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, "DELETE FROM command_receipts WHERE operation_id=$1", op)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, "DELETE FROM activelist_source_inbox WHERE source=$1", source)
		require.NoError(t, err)
	})

	flow.EnableDrain()
	packet.DeliveryID = "drain-delivery-098765"
	replay, err := flow.Push(ctx, packet)
	require.NoError(t, err)
	require.Equal(t, ack.StageCommitRef, replay.StageCommitRef)
	require.Equal(t, ack.PayloadSHA256, replay.PayloadSHA256)
	require.Equal(t, packet.DeliveryID, replay.DeliveryID)
	altered := packet
	altered.Event = json.RawMessage(strings.Replace(string(packet.Event), "original", "changed", 1))
	_, err = flow.Push(ctx, altered)
	require.ErrorIs(t, err, errcode.ErrConflict)
	unstaged := packet
	unstaged.Event = json.RawMessage(strings.ReplaceAll(strings.Replace(string(packet.Event), id, "drain-next-"+suffix, 1), `"cursor":"1"`, `"cursor":"2"`))
	_, err = flow.Push(ctx, unstaged)
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	var sourceCount, taskCount int
	require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM activelist_source_inbox WHERE source=$1", source).Scan(&sourceCount))
	require.NoError(t, testPool.QueryRow(ctx, "SELECT count(*) FROM command_receipts WHERE payload->>'type_name'=$1", cfg.TypeName).Scan(&taskCount))
	require.Equal(t, 1, sourceCount)
	require.Equal(t, 1, taskCount)
}

func TestB3DrainReconcilesWithoutSchedulingOrWriting(t *testing.T) {
	for _, tt := range []struct {
		name, status, reason string
		sent, recover        bool
		callback             bool
	}{
		{name: "unsubmitted", status: "held", reason: "integration_draining"},
		{name: "already_accepted", status: "held", reason: "integration_draining", callback: true},
		{name: "previously_sent_without_receipt", status: "pending", reason: "document_outcome_unconfirmed", sent: true},
		{name: "previously_sent_with_receipt", status: "succeeded", sent: true, recover: true, callback: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			grant := "drain-update-" + uniqueSuffix()
			op := b3protocol.Identity("op_ticket_", grant)
			taskID := b3protocol.TaskID(op)
			payload, err := json.Marshal(b3protocol.DocumentUpdate{
				TypeName: "sample", DocumentID: "7", Generation: "1", ExpectedRevision: "1",
				Patch: map[string]any{"name": "synthetic"}, GrantRef: grant, GrantRevision: "1",
			})
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO b3_grants(grant_ref,revision,subject_type,subject_id,type_name,generation,document_id,write_fields,enabled)
				VALUES ($1,1,'human','42','sample',1,7,ARRAY['name'],true)`, grant)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO command_receipts(operation_id,task_id,action,ticket_id,ticket_revision,node,
				definition_hash,action_index,actor_id,transition_roles,payload,remote_started)
				VALUES ($1,$2,'activelist.document.update',42,1,'assigned','synthetic',0,42,'{}',$3,$4)`, op, taskID, payload, tt.sent)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO business_intent_outbox(operation_id,accepted_at)
				VALUES ($1,CASE WHEN $2 THEN now() END)`, op, tt.callback)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := testPool.Exec(ctx, "DELETE FROM business_intent_outbox WHERE operation_id=$1", op)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, "DELETE FROM command_receipts WHERE operation_id=$1", op)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, "DELETE FROM b3_grants WHERE grant_ref=$1", grant)
				require.NoError(t, err)
			})

			tr := &b3TaskPeer{t: t, runs: make(map[string]json.RawMessage)}
			al := &b3RecoveryPeer{receiptAvailable: tt.recover}
			store := repository.NewB3Store(testPool)
			newDrainingFlow := func() *service.B3Flow {
				flow := service.NewB3Flow(testPool, store, tr, al, func(ctx context.Context, _ repository.B3Execution, _ string, execute func(context.Context) error) error {
					return execute(ctx)
				}, "", service.B3Subscription{})
				flow.EnableDrain()
				return flow
			}
			flow := newDrainingFlow()
			if tt.callback {
				params, err := json.Marshal(b3protocol.TaskParams{OperationID: op})
				require.NoError(t, err)
				callback := service.B3Callback{ProtocolVersion: 2, ActionVersion: 1, TaskID: taskID, Action: b3protocol.Update,
					OperationID: op, DeliveryID: "drain-update-delivery-012345", Params: params}
				receipt, err := flow.Callback(ctx, callback)
				if tt.recover {
					require.NoError(t, err)
					require.Equal(t, taskID, receipt.TaskID)
					require.Equal(t, op, receipt.OperationID)
					stored, err := flow.Status(ctx, taskID, op)
					require.NoError(t, err)
					require.Equal(t, receipt, stored)
				} else {
					require.ErrorIs(t, err, errcode.ErrForbidden)
				}
			} else {
				require.NoError(t, flow.Handle(ctx, nil))
			}
			run, err := store.Execution(ctx, taskID)
			require.NoError(t, err)
			require.Equal(t, tt.status, run.Status)
			require.Equal(t, tt.reason, run.Reason)
			require.Equal(t, tt.sent, run.RemoteStarted)
			if tt.sent {
				require.Equal(t, []string{op}, al.lookups)
			} else {
				require.Empty(t, al.lookups)
			}
			if tt.sent && !tt.recover {
				// A later scheduler must still reconcile the same pending operation.
				al.receiptAvailable = true
				_, err = testPool.Exec(ctx, "UPDATE business_intent_outbox SET next_attempt_at=now() WHERE operation_id=$1", op)
				require.NoError(t, err)
				flow = newDrainingFlow()
				require.NoError(t, flow.Handle(ctx, nil))
				receipt, err := flow.Status(ctx, taskID, op)
				require.NoError(t, err)
				require.Equal(t, "succeeded", receipt.State)
				require.Equal(t, []string{op, op}, al.lookups)
			}
			var accepted bool
			require.NoError(t, testPool.QueryRow(ctx, "SELECT accepted_at IS NOT NULL FROM business_intent_outbox WHERE operation_id=$1", op).Scan(&accepted))
			require.Equal(t, tt.callback, accepted)
			require.Empty(t, al.writes)
			require.Empty(t, tr.retried)
			require.Zero(t, tr.submissions)
		})
	}
}

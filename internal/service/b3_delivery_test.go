package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type b3DeliveryLedger struct {
	ak         string
	lookup     json.RawMessage
	response   *taskrunner.SubmitResponse
	submission taskrunner.SubmitRequest
}

func (l *b3DeliveryLedger) CallerAK() string { return l.ak }

func (l *b3DeliveryLedger) GetTask(context.Context, string) (json.RawMessage, error) {
	if l.lookup == nil {
		return nil, errcode.ErrNotFound
	}
	return l.lookup, nil
}

func (l *b3DeliveryLedger) Submit(_ context.Context, req taskrunner.SubmitRequest) (*taskrunner.SubmitResponse, error) {
	l.submission = req
	return l.response, nil
}

func (*b3DeliveryLedger) RetryTask(context.Context, string, string) (json.RawMessage, error) {
	panic("unexpected Taskrunner retry")
}

func TestB3DeliveryRecognizesOriginalLedgerRecord(t *testing.T) {
	operation := b3protocol.Identity("op_ticket_", "synthetic")
	d := repository.B3Delivery{OperationID: operation, TaskID: b3protocol.TaskID(operation), Action: b3protocol.Report}
	for _, tc := range []struct {
		name, actor, action, operation, jobID string
		matched                               bool
	}{
		{"historical operator", b3Submitter, d.Action, operation, "", true},
		{"configured service attribution", "service:synthetic-core", d.Action, operation, "", true},
		{"unrelated attribution", "service:other", d.Action, operation, "", false},
		{"unrelated action", b3Submitter, "ordinary.action", operation, "", false},
		{"unrelated operation", b3Submitter, d.Action, "other-operation-1234", "", false},
		{"scheduled task", b3Submitter, d.Action, operation, "a-scheduled-job", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			params, err := json.Marshal(b3protocol.TaskParams{OperationID: tc.operation})
			require.NoError(t, err)
			raw, err := json.Marshal(map[string]string{"task_id": d.TaskID, "action": tc.action,
				"params": string(params), "submitted_by": tc.actor, "status": "pending", "job_id": tc.jobID})
			require.NoError(t, err)
			ledger := &b3DeliveryLedger{ak: "synthetic-core", lookup: raw}
			require.Equal(t, tc.matched, (&B3Flow{tr: ledger}).deliver(context.Background(), d))
			require.Empty(t, ledger.submission.TaskID)
		})
	}
}

func TestB3DeliveryRequiresBoundNewAcceptance(t *testing.T) {
	operation := b3protocol.Identity("op_ticket_", "synthetic")
	d := repository.B3Delivery{OperationID: operation, TaskID: b3protocol.TaskID(operation), Action: b3protocol.Report}
	params, err := json.Marshal(b3protocol.TaskParams{OperationID: operation})
	require.NoError(t, err)
	for _, tc := range []struct {
		name, action string
		params       json.RawMessage
		accepted     bool
		matched      bool
	}{
		{"fresh durable acceptance", d.Action, params, true, true},
		{"duplicate awaits ledger lookup", d.Action, params, false, false},
		{"unbound acceptance", "", nil, true, false},
		{"operation mismatch", d.Action, json.RawMessage(`{"operation_id":"other-operation-1234"}`), true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := &b3DeliveryLedger{ak: "synthetic-core", response: &taskrunner.SubmitResponse{
				TaskID: d.TaskID, Action: tc.action, Params: tc.params, Accepted: tc.accepted,
			}}
			require.Equal(t, tc.matched, (&B3Flow{tr: ledger}).deliver(context.Background(), d))
			require.Equal(t, d.TaskID, ledger.submission.TaskID)
		})
	}
}

package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestD02AvailableActionAllowsEvidenceInputBeforeValidation(t *testing.T) {
	ticket := &model.Ticket{Status: "working", Revision: 7, CustomData: json.RawMessage(`{}`)}
	ttype := &model.TicketType{
		Transitions: json.RawMessage(`{"working":["review"]}`),
		TransitionMeta: json.RawMessage(`{"working>review":{
			"roles":["assignee"], "validator":"repair_plan != ''",
			"collect_fields":[{"key":"repair_plan","required":true}]}}`),
	}
	ref := &model.TicketDefinitionRef{TypeCode: "remediation", DefinitionVersion: 2}
	got, err := availableTicketActionContracts(context.Background(), ticket, ttype, ref, transitionActor{IsAssignee: true})
	require.NoError(t, err)
	require.Len(t, got, 1)
	require.Equal(t, "review", got[0].To)
	require.Equal(t, int64(7), got[0].ExpectedRevision)
	require.Equal(t, "repair_plan", got[0].CollectFields[0].Key)
	require.True(t, got[0].CollectFields[0].Required)

	got, err = availableTicketActionContracts(context.Background(), ticket, ttype, ref, transitionActor{})
	require.NoError(t, err)
	require.Empty(t, got)
}

package ticket

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReservedWithdrawalGraph(t *testing.T) {
	states := []string{"open", "assigned", "closed", "cancelled"}
	good := map[string][]string{"open": {"assigned", "cancelled"}, "assigned": {"closed"}}
	meta := json.RawMessage(`{"open>cancelled":{"roles":["creator"]}}`)
	require.NoError(t, ValidateTransitionMeta(states, good, meta))
	for _, tc := range []struct {
		name  string
		edges map[string][]string
		meta  json.RawMessage
	}{
		{"missing creator proof", good, nil},
		{"mixed authority", good, json.RawMessage(`{"open>cancelled":{"roles":["creator","admin"]}}`)},
		{"unfulfilled collection", good, json.RawMessage(`{"open>cancelled":{"roles":["creator"],"collect_fields":[{"key":"reason","required":true}]}}`)},
		{"unfulfilled delivery", good, json.RawMessage(`{"open>cancelled":{"roles":["creator"],"post_actions":["notify"]}}`)},
		{"reopen cancelled", map[string][]string{"open": {"cancelled"}, "cancelled": {"open"}}, meta},
		{"cancel from accepted", map[string][]string{"open": {"cancelled"}, "assigned": {"cancelled"}}, meta},
	} {
		t.Run(tc.name, func(t *testing.T) { require.Error(t, ValidateTransitionMeta(states, tc.edges, tc.meta)) })
	}
}

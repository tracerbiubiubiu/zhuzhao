package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func riskDispatchDefinitionFixture() (*model.TicketType, []*model.TicketTypeField) {
	return &model.TicketType{
		Code:           "synthetic_risk",
		States:         json.RawMessage(`["open","assigned","in_progress","pending_verify","closed"]`),
		Transitions:    json.RawMessage(`{"open":["assigned"],"assigned":["in_progress"],"in_progress":["pending_verify"],"pending_verify":["pending_verify","closed","in_progress"],"closed":[]}`),
		TransitionMeta: json.RawMessage(`{"assigned>in_progress":{"roles":["assignee"]},"in_progress>pending_verify":{"roles":["assignee"],"comment_required":true,"collect_fields":[{"key":"result","required":true}],"validator":"custom_data[\"result\"] == \"fixed\""},"pending_verify>pending_verify":{"roles":["creator"]},"pending_verify>closed":{"roles":["creator"]},"pending_verify>in_progress":{"roles":["creator"]}}`),
	}, []*model.TicketTypeField{{FieldKey: "result", FieldType: "input"}}
}

// Removing any of the real adapter's six consumed edges must prevent creation,
// including the self-loop used when only some assets have passed retest.
func TestRiskDispatchDefinitionRequiresEachConsumedEdge(t *testing.T) {
	for _, edge := range [][2]string{{"open", "assigned"}, {"assigned", "in_progress"}, {"in_progress", "pending_verify"}, {"pending_verify", "pending_verify"}, {"pending_verify", "closed"}, {"pending_verify", "in_progress"}} {
		t.Run(edge[0]+">"+edge[1], func(t *testing.T) {
			typ, fields := riskDispatchDefinitionFixture()
			var graph map[string][]string
			require.NoError(t, json.Unmarshal(typ.Transitions, &graph))
			var kept []string
			for _, to := range graph[edge[0]] {
				if to != edge[1] {
					kept = append(kept, to)
				}
			}
			graph[edge[0]] = kept
			typ.Transitions, _ = json.Marshal(graph)
			require.ErrorIs(t, ValidateRiskDispatchDefinition(typ, fields), errcode.ErrServiceUnavailable)
		})
	}
}

func TestRiskDispatchDefinitionRejectsUnusableRolesAndEffects(t *testing.T) {
	for _, tc := range []struct{ name, edge, meta string }{
		{"unassigned first actor", "open>assigned", `{"roles":["assignee"]}`},
		{"creator only accept", "assigned>in_progress", `{"roles":["creator"]}`},
		{"admin only submit", "in_progress>pending_verify", `{"roles":["admin"]}`},
		{"handler only partial retest", "pending_verify>pending_verify", `{"roles":["assignee"]}`},
		{"handler only pass", "pending_verify>closed", `{"roles":["assignee"]}`},
		{"handler only reject", "pending_verify>in_progress", `{"roles":["assignee"]}`},
		{"initial comment unavailable", "open>assigned", `{"comment_required":true}`},
		{"initial collection unavailable", "open>assigned", `{"collect_fields":[{"key":"result","required":false}]}`},
		{"initial post actions", "open>assigned", `{"post_actions":["notify"]}`},
		{"accept post actions", "assigned>in_progress", `{"post_actions":["notify"]}`},
		{"submit post actions", "in_progress>pending_verify", `{"post_actions":["notify"]}`},
		{"partial retest post actions", "pending_verify>pending_verify", `{"post_actions":["notify"]}`},
		{"pass post actions", "pending_verify>closed", `{"post_actions":["notify"]}`},
		{"reject post actions", "pending_verify>in_progress", `{"post_actions":["notify"]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typ, fields := riskDispatchDefinitionFixture()
			var meta map[string]json.RawMessage
			require.NoError(t, json.Unmarshal(typ.TransitionMeta, &meta))
			meta[tc.edge] = json.RawMessage(tc.meta)
			typ.TransitionMeta, _ = json.Marshal(meta)
			require.ErrorIs(t, ValidateRiskDispatchDefinition(typ, fields), errcode.ErrServiceUnavailable)
		})
	}
}

func TestRiskDispatchDefinitionRejectsUnsupportedCollection(t *testing.T) {
	for _, edge := range []string{"assigned>in_progress", "in_progress>pending_verify", "pending_verify>pending_verify", "pending_verify>closed", "pending_verify>in_progress"} {
		for _, fieldType := range []string{"file", "richtext", "tips", "missing", "managed", "formula"} {
			t.Run(edge+"/"+fieldType, func(t *testing.T) {
				typ, _ := riskDispatchDefinitionFixture()
				typ.TransitionMeta = json.RawMessage(`{"` + edge + `":{"collect_fields":[{"key":"result","required":false}]}}`)
				field := &model.TicketTypeField{FieldKey: "result", FieldType: fieldType}
				fields := []*model.TicketTypeField{field}
				switch fieldType {
				case "missing":
					fields = nil
				case "managed":
					field.FieldType = "input"
					field.FieldMeta = json.RawMessage(`{"write_authority":"risk_owner"}`)
				case "formula":
					field.FieldType = "input"
					field.FieldMeta = json.RawMessage(`{"formula":"1+1"}`)
				}
				require.ErrorIs(t, ValidateRiskDispatchDefinition(typ, fields), errcode.ErrServiceUnavailable)
			})
		}
	}
}

func TestRiskDispatchDefinitionAcceptsSupportedEvidenceAndUnrestrictedEdges(t *testing.T) {
	typ, fields := riskDispatchDefinitionFixture()
	require.NoError(t, ValidateRiskDispatchDefinition(typ, fields))
	typ.TransitionMeta = json.RawMessage(`{}`)
	require.NoError(t, ValidateRiskDispatchDefinition(typ, nil))
	typ.TransitionMeta = json.RawMessage(`{"open>assigned":{"roles":["creator","admin"]},"assigned>in_progress":{"roles":["creator","assignee"],"comment_required":true,"collect_fields":[{"key":"result","required":true}]}}`)
	require.NoError(t, ValidateRiskDispatchDefinition(typ, fields))
}

func TestRiskDispatchDefinitionRejectsMalformedSnapshot(t *testing.T) {
	require.ErrorIs(t, ValidateRiskDispatchDefinition(nil, nil), errcode.ErrServiceUnavailable)
	for _, part := range []string{"states", "graph", "meta", "nil field", "malformed field authority"} {
		t.Run(part, func(t *testing.T) {
			typ, fields := riskDispatchDefinitionFixture()
			switch part {
			case "states":
				typ.States = json.RawMessage(`["open"]`)
			case "graph":
				typ.Transitions = json.RawMessage(`{`)
			case "meta":
				typ.TransitionMeta = json.RawMessage(`{"assigned>in_progress":{"roles":["unknown"]}}`)
			case "nil field":
				fields = []*model.TicketTypeField{nil}
			case "malformed field authority":
				fields[0].FieldMeta = json.RawMessage(`{`)
			}
			require.ErrorIs(t, ValidateRiskDispatchDefinition(typ, fields), errcode.ErrServiceUnavailable)
		})
	}
}

func TestRiskDispatchInitialValidatorUsesComputedCandidate(t *testing.T) {
	p := syntheticPreparedCreate()
	p.ttype.TransitionMeta = json.RawMessage(`{"open>assigned":{"validator":"custom_data[\"computed\"] == 6"}}`)
	p.fields = []*model.TicketTypeField{
		{FieldKey: "base", FieldType: "number"},
		{FieldKey: "computed", FieldType: "number", FieldMeta: json.RawMessage(`{"formula":"base * 2"}`)},
	}
	p.ticket.CustomData = json.RawMessage(`{"base":3,"computed":999}`)
	s := &Service{delegation: initialRiskDelegation{}}
	ctx := context.WithValue(context.Background(), ticketCommandRolesKey{}, ticketCommandRoles{actor: p.actor})
	require.NoError(t, s.validateRiskDispatchInitialCandidate(ctx, nil, p, &p.ticket, 22))
	require.JSONEq(t, `{"base":3,"computed":999}`, string(p.ticket.CustomData), "preflight must not mutate the reusable prepared request")
}

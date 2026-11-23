package ticket

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// A missing binding, float conversion or independently normalized execution
// payload would allow changed transition input to replay another command.
func TestRiskWorkflowTicketInputBinding(t *testing.T) {
	in := RiskWorkflowTicketInput{DefinitionVersion: 7, CustomDataPatch: json.RawMessage(`{"z":9007199254740993,"a":"ok"}`), Comment: "review"}
	normal, hash, err := PrepareRiskWorkflowTicketInput(in)
	require.NoError(t, err)
	require.Equal(t, `{"a":"ok","z":9007199254740993}`, string(normal.CustomDataPatch))
	require.Regexp(t, `^[0-9a-f]{64}$`, hash)
	in.CustomDataPatch = json.RawMessage(` { "a":"ok", "z":9007199254740993 } `)
	_, same, err := PrepareRiskWorkflowTicketInput(in)
	require.NoError(t, err)
	require.Equal(t, hash, same)
	for _, field := range []string{"version", "patch", "comment"} {
		changed := in
		switch field {
		case "version":
			changed.DefinitionVersion++
		case "patch":
			changed.CustomDataPatch = json.RawMessage(`{"a":"ok","z":9007199254740992}`)
		case "comment":
			changed.Comment += "!"
		}
		_, other, err := PrepareRiskWorkflowTicketInput(changed)
		require.NoError(t, err)
		require.NotEqual(t, hash, other, field)
	}
	for _, raw := range []string{"null", "[]", `{"x":1} {}`, "invalid", strings.Repeat(" ", CustomDataMaxBytes+1)} {
		_, _, err := PrepareRiskWorkflowTicketInput(RiskWorkflowTicketInput{DefinitionVersion: 7, CustomDataPatch: json.RawMessage(raw)})
		require.Error(t, err, raw[:min(len(raw), 30)])
	}
	_, empty, err := PrepareRiskWorkflowTicketInput(RiskWorkflowTicketInput{DefinitionVersion: 7})
	require.NoError(t, err)
	_, object, err := PrepareRiskWorkflowTicketInput(RiskWorkflowTicketInput{DefinitionVersion: 7, CustomDataPatch: json.RawMessage(`{}`)})
	require.NoError(t, err)
	require.Equal(t, empty, object)
	_, _, err = PrepareRiskWorkflowTicketInput(RiskWorkflowTicketInput{})
	require.Error(t, err)
	_, _, err = PrepareRiskWorkflowTicketInput(RiskWorkflowTicketInput{DefinitionVersion: 7, Comment: strings.Repeat("x", RichtextMaxBytes+1)})
	require.Error(t, err)
}

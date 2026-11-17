package msgrender

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestSavedBuiltinVariablesAreRequiredAtRender(t *testing.T) {
	for _, key := range []string{"ticket.type_code", "ticket.priority", "ticket.actor", "ticket.creator", "ticket.to_status", "ticket.to_assignee", "ticket.reminder_count", "event.error", "event.level"} {
		t.Run(key, func(t *testing.T) {
			result := RenderQuadrant("{{"+key+"}}", map[string]any{})
			require.Error(t, result.Err, "a supported builtin cannot silently degrade to an empty dynamic field")
		})
	}
}

package model

import (
	"encoding/json"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestD01VersionsUseDecimalStrings(t *testing.T) {
	version := int64(9007199254740993)
	for _, tc := range []struct {
		value any
		key   string
	}{
		{Ticket{DefinitionVersion: &version}, "definition_version"},
		{TicketType{CurrentPublishedVersion: &version}, "current_published_version"},
		{TicketTypeDefinition{DefinitionVersion: version}, "definition_version"},
		{TicketTypeDefinition{SourceDraftRevision: version}, "source_draft_revision"},
	} {
		raw, err := json.Marshal(tc.value)
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(raw, &got))
		require.Equal(t, "9007199254740993", got[tc.key], tc.key)
	}
}

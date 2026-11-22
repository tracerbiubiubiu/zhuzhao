package model

import (
	"encoding/json"
	"time"
)

// TicketTypeDefinition is an append-only published rule set, separate from the draft.
type TicketTypeDefinition struct {
	TypeCode            string          `json:"type_code"`
	DefinitionVersion   int64           `json:"definition_version,string"`
	DefinitionHash      string          `json:"definition_hash"`
	SchemaFormatVersion int             `json:"schema_format_version"`
	Snapshot            json.RawMessage `json:"snapshot"`
	SourceDraftRevision int64           `json:"source_draft_revision,string"`
	PublishedBy         *int64          `json:"published_by,string"` // nil for the v83 cutover baseline
	PublishedAt         time.Time       `json:"published_at"`
}

// TicketDefinitionSnapshot reuses the existing graph/field contract. Row IDs and
// draft revision are not definition semantics; FieldID is the stable identity.
type TicketDefinitionSnapshot struct {
	Type   TicketType         `json:"type"`
	Fields []*TicketTypeField `json:"fields"`
}

package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// BoundTicketDefinition resolves the recorded rule identity on the caller's
// command connection. A draft or current pointer cannot fill a missing binding.
func (r *TicketRepo) BoundTicketDefinition(ctx context.Context, code string, version *int64) (*model.TicketDefinitionSnapshot, *model.TicketDefinitionRef, error) {
	return boundTicketDefinition(ctx, IAMDB(ctx, r.db), code, version)
}

func boundTicketDefinition(ctx context.Context, q rowExec, code string, version *int64) (*model.TicketDefinitionSnapshot, *model.TicketDefinitionRef, error) {
	if version == nil {
		return nil, nil, errcode.ErrServiceUnavailable
	}
	var raw json.RawMessage
	ref := model.TicketDefinitionRef{TypeCode: code, DefinitionVersion: *version}
	err := q.QueryRow(ctx, `SELECT definition_hash,snapshot FROM ticket_type_definitions
		WHERE type_code=$1 AND definition_version=$2 AND schema_format_version=1
		  AND definition_hash=ticket_definition_snapshot_hash(snapshot)`, code, *version).Scan(&ref.DefinitionHash, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errcode.ErrServiceUnavailable
	}
	if err != nil {
		return nil, nil, fmt.Errorf("load bound ticket definition: %w", err)
	}
	snapshot, err := DecodeBoundTicketDefinition(code, raw)
	if err != nil {
		return nil, nil, err
	}
	return snapshot, &ref, nil
}

// BoundTicketReminderRules identifies the reminder policy shared by tickets on one definition.
type BoundTicketReminderRules struct {
	TypeCode          string
	DefinitionVersion int64
	TransitionMeta    json.RawMessage
}

// ListBoundTicketReminderRules returns usable definitions referenced by tickets.
// Scans match both the type and version using the existing ticket index.
func (r *TicketRepo) ListBoundTicketReminderRules(ctx context.Context) ([]BoundTicketReminderRules, error) {
	rows, err := IAMDB(ctx, r.db).Query(ctx, `SELECT d.type_code,d.definition_version,d.snapshot
		FROM ticket_type_definitions d
		WHERE d.schema_format_version=1 AND d.definition_hash=ticket_definition_snapshot_hash(d.snapshot)
		  AND jsonb_typeof(d.snapshot #> '{type,transition_meta}')='object'
		  AND d.snapshot #> '{type,transition_meta}' <> '{}'::jsonb
		  AND EXISTS (SELECT 1 FROM tickets t WHERE t.type_code=d.type_code AND t.definition_version=d.definition_version)`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var rules []BoundTicketReminderRules
	for rows.Next() {
		var rule BoundTicketReminderRules
		var raw json.RawMessage
		if err := rows.Scan(&rule.TypeCode, &rule.DefinitionVersion, &raw); err != nil {
			return nil, err
		}
		snapshot, err := DecodeBoundTicketDefinition(rule.TypeCode, raw)
		if err != nil {
			continue
		}
		rule.TransitionMeta = snapshot.Type.TransitionMeta
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// DecodeBoundTicketDefinition checks the format-1 document shared by runtime
// resolution and trusted legacy binding. Callers verify the stored identity
// and hash before decoding.
func DecodeBoundTicketDefinition(code string, raw json.RawMessage) (*model.TicketDefinitionSnapshot, error) {
	var document struct {
		Type struct {
			model.TicketType
			DefaultSLAHours *int  `json:"default_sla_hours"`
			HasCustomFields *bool `json:"has_custom_fields"`
		} `json:"type"`
		Fields []*struct {
			model.TicketTypeField
			Required  *bool `json:"required"`
			SortOrder *int  `json:"sort_order"`
		} `json:"fields"`
	}
	if json.Unmarshal(raw, &document) != nil || document.Type.Code != code ||
		document.Type.DefaultSLAHours == nil || document.Type.HasCustomFields == nil || document.Fields == nil {
		return nil, errcode.ErrServiceUnavailable
	}
	ticketType := document.Type.TicketType
	ticketType.DefaultSLAHours = *document.Type.DefaultSLAHours
	ticketType.HasCustomFields = *document.Type.HasCustomFields
	if !usableTicketDefinitionGraph(ticketType) {
		return nil, errcode.ErrServiceUnavailable
	}

	snapshot := model.TicketDefinitionSnapshot{
		Type:   ticketType,
		Fields: make([]*model.TicketTypeField, 0, len(document.Fields)),
	}
	fieldIDs := make(map[int64]bool, len(document.Fields))
	fieldKeys := make(map[string]bool, len(document.Fields))
	for _, entry := range document.Fields {
		if entry == nil || entry.TypeCode != code || entry.FieldID <= 0 || entry.FieldKey == "" ||
			entry.Required == nil || entry.SortOrder == nil || fieldIDs[entry.FieldID] || fieldKeys[entry.FieldKey] ||
			!supportedDefinitionFieldType(entry.FieldType) {
			return nil, errcode.ErrServiceUnavailable
		}
		field := entry.TicketTypeField
		field.Required = *entry.Required
		field.SortOrder = *entry.SortOrder
		snapshot.Fields = append(snapshot.Fields, &field)
		fieldIDs[field.FieldID], fieldKeys[field.FieldKey] = true, true
	}
	return &snapshot, nil
}

func usableTicketDefinitionGraph(ticketType model.TicketType) bool {
	var states []string
	var transitions map[string][]string
	var metadata map[string]json.RawMessage
	if json.Unmarshal(ticketType.States, &states) != nil || json.Unmarshal(ticketType.Transitions, &transitions) != nil ||
		transitions == nil || json.Unmarshal(ticketType.TransitionMeta, &metadata) != nil {
		return false
	}
	knownStates := make(map[string]bool, len(states))
	for _, state := range states {
		if state == "" || strings.TrimSpace(state) != state || knownStates[state] {
			return false
		}
		knownStates[state] = true
	}
	if !knownStates["open"] || !knownStates["assigned"] || !knownStates["closed"] {
		return false
	}
	knownEdges := make(map[string]bool)
	for from, targets := range transitions {
		if !knownStates[from] || targets == nil {
			return false
		}
		for _, target := range targets {
			if !knownStates[target] {
				return false
			}
			knownEdges[from+">"+target] = true
		}
	}
	for key, raw := range metadata {
		if strings.Contains(key, ">") {
			if !knownEdges[key] {
				return false
			}
		} else if !knownStates[key] {
			return false
		}
		var segment map[string]json.RawMessage
		if json.Unmarshal(raw, &segment) != nil || segment == nil {
			return false
		}
	}
	return true
}

// Format-1 accepts every field type supported by TicketTypeFieldInput and the
// UI, including tips. Keep the v83 cutover filter aligned with this set.
func supportedDefinitionFieldType(fieldType string) bool {
	switch fieldType {
	case "input", "textarea", "number", "date", "select", "multi_select", "tips", "richtext", "radio", "rating", "file":
		return true
	default:
		return false
	}
}

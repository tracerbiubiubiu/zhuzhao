package ticket

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// PublishTicketType applies a reviewed configuration in the enclosing command.
// HTTP callers enter through ExecuteTicketDefinitionPublication.
// expected is the reviewed draft identity, revision and publication pointer.
func (s *Service) PublishTicketType(ctx context.Context, expected *model.TicketType, actor int64) (*model.TicketTypeDefinition, bool, error) {
	if expected == nil || expected.ID <= 0 || expected.Version <= 0 || actor <= 0 {
		return nil, false, errcode.ErrInvalidParams
	}
	var result *model.TicketTypeDefinition
	changed := false
	err := repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		draft, fields, err := s.ticketRepo.LockDefinitionDraftTx(ctx, tx, expected)
		if err != nil {
			return err
		}
		if err = validateTypeGraph(draft.States, draft.Transitions); err != nil {
			return err
		}
		var states []string
		if err = json.Unmarshal(draft.States, &states); err != nil {
			return err
		}
		if err = validateTransitionMeta(states, draft.TransitionMeta); err != nil {
			return err
		}
		var edges map[string][]string
		if err = json.Unmarshal(draft.Transitions, &edges); err != nil {
			return err
		}
		if err = validateTransitionEdges(states, edges, draft.TransitionMeta, s.actionReg.validatePostActions); err != nil {
			return err
		}
		inputs := make([]model.TicketTypeFieldInput, 0, len(fields))
		for _, f := range fields {
			if f.FieldID <= 0 {
				return errcode.ErrServiceUnavailable
			}

			// The draft editor persists absent options as []. Its input-only
			// numeric validation expects omission; adapt only that default,
			// preserving the actual stored snapshot and rejecting real options.
			options := f.FieldOptions
			if (f.FieldType == "number" || f.FieldType == "rating") && sameJSONConfig(options, json.RawMessage(`[]`)) {
				options = nil
			}
			inputs = append(inputs, model.TicketTypeFieldInput{FieldKey: f.FieldKey, FieldLabel: f.FieldLabel, FieldType: f.FieldType, FieldOptions: options, FieldMeta: f.FieldMeta, Required: f.Required, ValidateRegex: f.ValidateRegex, SortOrder: f.SortOrder})
			f.ID = 0 // transient replacement row identity is not definition content
		}
		if _, err = validateFieldInputs(inputs); err != nil {
			return err
		}
		if err = validatePublishedEvidenceSpec(draft.TransitionMeta, fields); err != nil {
			return err
		}
		if err = s.ticketRepo.LockDefinitionDictReferencesTx(ctx, tx, fields); err != nil {
			return err
		}
		// Runtime identity, live activation and draft revision are not rule content.
		snap := *draft
		snap.ID = 0
		snap.Version = 0
		snap.IsActive = false
		snap.CurrentPublishedVersion = nil
		snap.CreatedAt = time.Time{}
		raw, err := json.Marshal(model.TicketDefinitionSnapshot{Type: snap, Fields: fields})
		if err != nil {
			return err
		}
		hash, err := s.ticketRepo.DefinitionSnapshotHashTx(ctx, tx, raw)
		if err != nil {
			return err
		}
		next := int64(1)
		if draft.CurrentPublishedVersion != nil {
			current, err := s.ticketRepo.GetTicketDefinitionTx(ctx, tx, draft.Code, *draft.CurrentPublishedVersion)
			if err != nil {
				return err
			}
			if current.DefinitionHash == hash {
				result = current
				return nil
			}
			next = *draft.CurrentPublishedVersion + 1
		}
		result = &model.TicketTypeDefinition{TypeCode: draft.Code, DefinitionVersion: next, DefinitionHash: hash, SchemaFormatVersion: 1, Snapshot: raw, SourceDraftRevision: draft.Version, PublishedBy: &actor}
		if err = s.ticketRepo.AppendTicketDefinitionTx(ctx, tx, result); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return nil, false, err
	}
	return result, changed, nil
}

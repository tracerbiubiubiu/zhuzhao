package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// LockDefinitionDraftTx serializes draft updates, field replacement, publication
// and deletion on the same parent row. The caller owns the IAM guard and tx.
func (r *TicketRepo) LockDefinitionDraftTx(ctx context.Context, tx pgx.Tx, expected *model.TicketType) (*model.TicketType, []*model.TicketTypeField, error) {
	var t model.TicketType
	err := tx.QueryRow(ctx, `SELECT id,code,name,COALESCE(description,''),states,transitions,
 COALESCE(transition_meta,'{}'::jsonb),default_sla_hours,has_custom_fields,is_active,version,created_at,current_published_version
 FROM ticket_types WHERE code=$1 FOR UPDATE`, expected.Code).Scan(&t.ID, &t.Code, &t.Name, &t.Description, &t.States, &t.Transitions, &t.TransitionMeta, &t.DefaultSLAHours, &t.HasCustomFields, &t.IsActive, &t.Version, &t.CreatedAt, &t.CurrentPublishedVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, errcode.ErrTicketTypeNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if t.ID != expected.ID || t.Version != expected.Version || !samePublishedVersion(t.CurrentPublishedVersion, expected.CurrentPublishedVersion) {
		return nil, nil, errcode.ErrConcurrentModification
	}
	if !t.IsActive {
		return nil, nil, errcode.ErrTicketTypeNotFound
	}
	fields, err := listTicketTypeFields(ctx, tx, t.Code)
	return &t, fields, err
}

func samePublishedVersion(a, b *int64) bool {
	return a == nil && b == nil || a != nil && b != nil && *a == *b
}

func (r *TicketRepo) DefinitionSnapshotHashTx(ctx context.Context, tx pgx.Tx, snapshot json.RawMessage) (string, error) {
	var hash string
	err := tx.QueryRow(ctx, `SELECT ticket_definition_snapshot_hash($1::jsonb)`, snapshot).Scan(&hash)
	return hash, err
}

func (r *TicketRepo) GetTicketDefinitionTx(ctx context.Context, q rowExec, code string, version int64) (*model.TicketTypeDefinition, error) {
	var d model.TicketTypeDefinition
	err := q.QueryRow(ctx, `SELECT type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by,published_at FROM ticket_type_definitions WHERE type_code=$1 AND definition_version=$2`, code, version).Scan(&d.TypeCode, &d.DefinitionVersion, &d.DefinitionHash, &d.SchemaFormatVersion, &d.Snapshot, &d.SourceDraftRevision, &d.PublishedBy, &d.PublishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrServiceUnavailable
	}
	return &d, err
}

// AppendTicketDefinitionTx must follow LockDefinitionDraftTx in the same tx.
// The definition row itself is the durable publication fact; public plan and
// operation receipts are intentionally not opened by this storage slice.
func (r *TicketRepo) AppendTicketDefinitionTx(ctx context.Context, tx pgx.Tx, d *model.TicketTypeDefinition) error {
	if err := tx.QueryRow(ctx, `INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING published_at`, d.TypeCode, d.DefinitionVersion, d.DefinitionHash, d.SchemaFormatVersion, d.Snapshot, d.SourceDraftRevision, d.PublishedBy).Scan(&d.PublishedAt); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE ticket_types SET current_published_version=$2 WHERE code=$1`, d.TypeCode, d.DefinitionVersion)
	return err
}

// PublishedCreateDefinition resolves the pointer and immutable contents in one
// statement; no second live draft read may become validation input.
func (r *TicketRepo) PublishedCreateDefinition(ctx context.Context, code string) (*model.TicketType, []*model.TicketTypeField, error) {
	var id int64
	var active bool
	var version *int64
	var raw json.RawMessage
	var format *int
	err := IAMDB(ctx, r.db).QueryRow(ctx, `SELECT t.id,t.is_active,t.current_published_version,d.snapshot,d.schema_format_version
 FROM ticket_types t LEFT JOIN ticket_type_definitions d ON d.type_code=t.code AND d.definition_version=t.current_published_version WHERE t.code=$1`, code).Scan(&id, &active, &version, &raw, &format)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !active {
		return nil, nil, errcode.ErrTicketTypeNotFound
	}
	if err != nil {
		return nil, nil, err
	}
	if version == nil || len(raw) == 0 || format == nil || *format != 1 {
		return nil, nil, errcode.ErrServiceUnavailable
	}
	snap, err := DecodeBoundTicketDefinition(code, raw)
	if err != nil {
		return nil, nil, err
	}
	snap.Type.ID = id
	snap.Type.IsActive = active
	snap.Type.CurrentPublishedVersion = version
	return &snap.Type, snap.Fields, nil
}

// LockPublishedCreateTx checks the preflight pointer under a shared parent lock,
// held through ticket insert/commit. Draft-only edits do not invalidate create.
func (r *TicketRepo) LockPublishedCreateTx(ctx context.Context, tx pgx.Tx, expected *model.TicketType, templateCode string, template *model.TicketTemplate) error {
	var id int64
	var active bool
	var version *int64
	err := tx.QueryRow(ctx, `SELECT id,is_active,current_published_version FROM ticket_types WHERE code=$1 FOR SHARE`, expected.Code).Scan(&id, &active, &version)
	if errors.Is(err, pgx.ErrNoRows) || err == nil && !active {
		return errcode.ErrTicketTypeNotFound
	}
	if err != nil {
		return err
	}
	if id != expected.ID || !samePublishedVersion(version, expected.CurrentPublishedVersion) {
		return errcode.ErrConcurrentModification
	}
	if version == nil {
		return errcode.ErrServiceUnavailable
	}
	if templateCode == "" {
		return nil
	}
	var now model.TicketTemplate
	err = tx.QueryRow(ctx, `SELECT id,version,type_code,default_priority,default_fields FROM ticket_templates WHERE code=$1 AND deleted_at IS NULL FOR SHARE`, templateCode).Scan(&now.ID, &now.Version, &now.TypeCode, &now.DefaultPriority, &now.DefaultFields)
	if errors.Is(err, pgx.ErrNoRows) {
		if template != nil {
			return errcode.ErrConcurrentModification
		}
		return nil
	}
	if err != nil {
		return err
	}
	if template == nil || now.ID != template.ID || now.Version != template.Version || now.TypeCode != template.TypeCode || now.DefaultPriority != template.DefaultPriority || string(now.DefaultFields) != string(template.DefaultFields) {
		return errcode.ErrConcurrentModification
	}
	return nil
}

// LockDefinitionDictReferencesTx preserves the identity of live dictionary
// references while publishing. Delete takes FOR UPDATE then rescans both draft
// and published references, using the same ordering as field replacement.
func (r *TicketRepo) LockDefinitionDictReferencesTx(ctx context.Context, tx pgx.Tx, fields []*model.TicketTypeField) error {
	values := make([]model.TicketTypeField, 0, len(fields))
	for _, f := range fields {
		values = append(values, *f)
	}
	for _, ref := range dictFieldReferences(values) {
		var code string
		err := tx.QueryRow(ctx, `SELECT code FROM dict_types WHERE code=$1 FOR KEY SHARE`, ref.code).Scan(&code)
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrConcurrentModification
		}
		if err != nil {
			return err
		}
	}
	return nil
}

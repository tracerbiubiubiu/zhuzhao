package repository

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type DefinitionPublicationPlan struct {
	model.TicketDefinitionPlan
	ActorID, TypeID          int64
	TypeCode                 string
	ExpectedPublishedVersion *int64
	SnapshotHash             string
}

func (r *TicketRepo) InsertDefinitionPublicationPlan(ctx context.Context, tx pgx.Tx, p *DefinitionPublicationPlan) error {
	return tx.QueryRow(ctx, `INSERT INTO ticket_definition_publication_plans
		(actor_id,type_id,type_code,draft_revision,expected_published_version,canonical_hash,snapshot_hash,reference_revisions,warnings)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, p.ActorID, p.TypeID, p.TypeCode, p.DraftRevision,
		p.ExpectedPublishedVersion, p.CanonicalHash, p.SnapshotHash, p.ReferenceRevisions, p.Warnings).Scan(&p.PlanID)
}

func (r *TicketRepo) GetDefinitionPublicationPlan(ctx context.Context, tx pgx.Tx, actor, id int64) (*DefinitionPublicationPlan, error) {
	p := DefinitionPublicationPlan{ActorID: actor}
	err := tx.QueryRow(ctx, `SELECT id,type_id,type_code,draft_revision,expected_published_version,canonical_hash,snapshot_hash,reference_revisions,warnings
		FROM ticket_definition_publication_plans WHERE actor_id=$1 AND id=$2`, actor, id).Scan(&p.PlanID, &p.TypeID, &p.TypeCode,
		&p.DraftRevision, &p.ExpectedPublishedVersion, &p.CanonicalHash, &p.SnapshotHash, &p.ReferenceRevisions, &p.Warnings)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrConcurrentModification
	}
	return &p, err
}

// LockDefinitionPublicationOperation serializes a caller's K even when two
// requests bearing it target different types. Always acquire it before type locks.
func (r *TicketRepo) LockDefinitionPublicationOperation(ctx context.Context, tx pgx.Tx, actor int64, operationID string) error {
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(591815184,hashtext($1::bigint::text || ':' || $2::text))`, actor, operationID)
	return err
}

func (r *TicketRepo) GetDefinitionPublicationReceipt(ctx context.Context, tx pgx.Tx, actor int64, operationID string) (*model.TicketDefinitionPublicationReceipt, string, error) {
	receipt := model.TicketDefinitionPublicationReceipt{OperationID: operationID}
	var requestHash string
	err := tx.QueryRow(ctx, `SELECT request_hash,type_code,definition_version,definition_hash,draft_revision,changed
		FROM ticket_definition_publication_receipts WHERE actor_id=$1 AND operation_id=$2`, actor, operationID).
		Scan(&requestHash, &receipt.DefinitionRef.TypeCode, &receipt.DefinitionRef.DefinitionVersion, &receipt.DefinitionRef.DefinitionHash, &receipt.DraftRevision, &receipt.Changed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", nil
	}
	return &receipt, requestHash, err
}

func (r *TicketRepo) InsertDefinitionPublicationReceipt(ctx context.Context, tx pgx.Tx, actor, planID int64, requestHash string, receipt *model.TicketDefinitionPublicationReceipt) error {
	_, err := tx.Exec(ctx, `INSERT INTO ticket_definition_publication_receipts
		(actor_id,operation_id,request_hash,plan_id,type_code,definition_version,definition_hash,draft_revision,changed)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`, actor, receipt.OperationID, requestHash, planID, receipt.DefinitionRef.TypeCode,
		receipt.DefinitionRef.DefinitionVersion, receipt.DefinitionRef.DefinitionHash, receipt.DraftRevision, receipt.Changed)
	return err
}

// Dictionary option values are live. Bind the referenced dictionary type's
// identity and revision, holding its row through the plan or publication
// commit so an update or delete cannot pass between check and write.
func (r *TicketRepo) LockDefinitionReferenceRevisions(ctx context.Context, tx pgx.Tx, fields []*model.TicketTypeField, expressionDictionaries map[string]struct{}, notificationTemplates []string) (map[string]model.TicketDefinitionReferenceRevision, error) {
	values := make([]model.TicketTypeField, 0, len(fields))
	for _, f := range fields {
		values = append(values, *f)
	}
	dictionaries := maps.Clone(expressionDictionaries)
	for _, ref := range dictFieldReferences(values) {
		dictionaries[ref.code] = struct{}{}
	}
	revisions := make(map[string]model.TicketDefinitionReferenceRevision)
	for _, code := range slices.Sorted(maps.Keys(dictionaries)) {
		var rev model.TicketDefinitionReferenceRevision
		err := tx.QueryRow(ctx, `SELECT id,version FROM dict_types WHERE code=$1 FOR SHARE`, code).Scan(&rev.ID, &rev.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrConcurrentModification
		}
		if err != nil {
			return nil, err
		}
		revisions["dict:"+code] = rev
	}
	for _, code := range notificationTemplates {
		var rev model.TicketDefinitionReferenceRevision
		err := tx.QueryRow(ctx, `SELECT id,version FROM message_templates WHERE code=$1 AND biz_type='ticket' AND enabled FOR SHARE`, code).Scan(&rev.ID, &rev.Version)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, errcode.ErrConcurrentModification
		}
		if err != nil {
			return nil, err
		}
		revisions["message_template:"+code] = rev
	}
	return revisions, nil
}

func (r *TicketRepo) HasDefinitionConfigMenu(ctx context.Context, actor int64, publishing bool) (bool, error) {
	var allowed bool
	err := IAMDB(ctx, r.db).QueryRow(ctx, EffectiveRoleCTE()+`SELECT EXISTS (
		SELECT 1 FROM expanded e JOIN role_menus rm ON rm.role_id=e.id JOIN menus m ON m.id=rm.menu_id
		WHERE m.deleted_at IS NULL AND m.is_system AND (
			(m.code='ticket_definition_publish_btn' AND m.menu_type=3 AND m.permission='ticket:type:publish') OR
			(NOT $2 AND m.permission='ticket:type:manage' AND (
				(m.code='ticket_type_manage' AND m.menu_type=2) OR (m.code='ticket_type_write_btn' AND m.menu_type=3)))))`,
		actor, publishing).Scan(&allowed)
	return allowed, err
}

// CurrentDefinitionForForm reads only the live admission bit and the immutable
// current pointer/snapshot in one statement. A nil definition is unpublished.
func (r *TicketRepo) CurrentDefinitionForForm(ctx context.Context, tx pgx.Tx, code string) (*model.TicketTypeDefinition, error) {
	var def model.TicketTypeDefinition
	var version *int64
	var hash *string
	var format *int
	err := tx.QueryRow(ctx, `SELECT t.current_published_version,d.definition_hash,d.schema_format_version,d.snapshot
		FROM ticket_types t LEFT JOIN ticket_type_definitions d ON (d.type_code,d.definition_version)=(t.code,t.current_published_version)
		WHERE t.code=$1 AND t.is_active`, code).Scan(&version, &hash, &format, &def.Snapshot)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrTicketTypeNotFound
	}
	if err != nil || version == nil {
		return nil, err
	}
	if hash == nil || format == nil {
		return nil, errcode.ErrServiceUnavailable
	}
	def.TypeCode, def.DefinitionVersion, def.DefinitionHash, def.SchemaFormatVersion = code, *version, *hash, *format
	return &def, nil
}

// ListPublishedDefinitionTypes serves the legacy list used for ticket filters.
// Keep its public shape while omitting the internal transition metadata.
func (r *TicketRepo) ListPublishedDefinitionTypes(ctx context.Context, tx pgx.Tx) ([]*model.TicketType, error) {
	rows, err := tx.Query(ctx, `SELECT t.id,t.code,t.created_at,d.definition_version,d.snapshot->'type'
		FROM ticket_types t JOIN ticket_type_definitions d ON (d.type_code,d.definition_version)=(t.code,t.current_published_version)
		WHERE t.is_active AND d.schema_format_version=1 ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketType, error) {
		var id, version int64
		var code string
		var createdAt time.Time
		var raw json.RawMessage
		if err := row.Scan(&id, &code, &createdAt, &version, &raw); err != nil {
			return nil, err
		}
		var t model.TicketType
		if err := json.Unmarshal(raw, &t); err != nil || t.Code != code {
			return nil, errcode.ErrServiceUnavailable
		}
		t.ID, t.CreatedAt, t.IsActive, t.CurrentPublishedVersion = id, createdAt, true, &version
		t.TransitionMeta = nil
		return &t, nil
	})
}

func (r *TicketRepo) ListDefinitionDraftTypes(ctx context.Context, tx pgx.Tx) ([]*model.TicketType, error) {
	rows, err := tx.Query(ctx, `SELECT id,code,name,COALESCE(description,''),states,transitions,
		COALESCE(transition_meta,'{}'::jsonb),default_sla_hours,has_custom_fields,is_active,version,created_at,current_published_version
		FROM ticket_types WHERE is_active ORDER BY id`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.TicketType, error) {
		var t model.TicketType
		err := row.Scan(&t.ID, &t.Code, &t.Name, &t.Description, &t.States, &t.Transitions, &t.TransitionMeta,
			&t.DefaultSLAHours, &t.HasCustomFields, &t.IsActive, &t.Version, &t.CreatedAt, &t.CurrentPublishedVersion)
		return &t, err
	})
}

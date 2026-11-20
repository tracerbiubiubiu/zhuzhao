package repository

import (
	"bytes"
	"context"
	"errors"
	"reflect"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// LockCreateSnapshotTx fences the configuration actually used during create
// preparation. Call after the IAM guard, before org/ticket/file locks. Type
// writers (including field replacement) lock the parent first; all acquired
// locks remain held through the command's commit. Compare decision data as
// well as identity/version so unversioned configuration edits cannot slip in.
func (r *TicketRepo) LockCreateSnapshotTx(ctx context.Context, tx pgx.Tx, expected *model.TicketType, templateCode string, template *model.TicketTemplate, fields []*model.TicketTypeField) error {
	var current model.TicketType
	err := tx.QueryRow(ctx, `SELECT id,version,is_active,states,transitions,
		COALESCE(transition_meta,'{}'::jsonb),default_sla_hours,has_custom_fields
		FROM ticket_types WHERE code=$1 FOR SHARE`, expected.Code).Scan(
		&current.ID, &current.Version, &current.IsActive, &current.States, &current.Transitions,
		&current.TransitionMeta, &current.DefaultSLAHours, &current.HasCustomFields)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !current.IsActive) {
		return errcode.ErrTicketTypeNotFound
	}
	if err != nil {
		return err
	}
	if current.ID != expected.ID || current.Version != expected.Version ||
		!bytes.Equal(current.States, expected.States) || !bytes.Equal(current.Transitions, expected.Transitions) ||
		!bytes.Equal(current.TransitionMeta, expected.TransitionMeta) || current.DefaultSLAHours != expected.DefaultSLAHours ||
		current.HasCustomFields != expected.HasCustomFields {
		return errcode.ErrConcurrentModification
	}
	if templateCode != "" {
		var now model.TicketTemplate
		err := tx.QueryRow(ctx, `SELECT id,version,type_code,default_priority,default_fields
			FROM ticket_templates WHERE code=$1 AND deleted_at IS NULL FOR SHARE`, templateCode).Scan(
			&now.ID, &now.Version, &now.TypeCode, &now.DefaultPriority, &now.DefaultFields)
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			// An optional template that was absent may still be absent. A
			// template used for preparation must never silently disappear.
			if template != nil {
				return errcode.ErrConcurrentModification
			}
		case err != nil:
			return err
		default:
			if template == nil || now.ID != template.ID || now.Version != template.Version ||
				now.TypeCode != template.TypeCode || now.DefaultPriority != template.DefaultPriority ||
				!bytes.Equal(now.DefaultFields, template.DefaultFields) {
				return errcode.ErrConcurrentModification
			}
		}
	}
	currentFields, err := listTicketTypeFields(ctx, tx, expected.Code, true)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(currentFields, fields) {
		return errcode.ErrConcurrentModification
	}
	return nil
}

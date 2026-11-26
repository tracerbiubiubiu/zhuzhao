package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
)

// RecordMenuAudit appends to the transaction that owns the menu replacement.
func (r *RoleRepo) RecordMenuAudit(ctx context.Context, actor, target int64, previous, current []int64, collector string) error {
	state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState)
	if !ok || state.shared || state.pool != r.db {
		return errors.New("IAM menu audit requires the owning write transaction")
	}
	return RecordIAMFactTx(ctx, state.tx, "iam.role.menus.set", actor, target, previous, current, collector)
}

func RecordIAMFactTx(ctx context.Context, tx pgx.Tx, event string, actor, target int64, previous, current []int64, collector string) error {
	var id string
	err := tx.QueryRow(ctx, `
        INSERT INTO iam_audit_facts(event_name, actor_id, target_id, added_ids, removed_ids)
        VALUES ($1, $2, $3,
            ARRAY(SELECT unnest($5::bigint[]) EXCEPT SELECT unnest($4::bigint[]) ORDER BY 1),
            ARRAY(SELECT unnest($4::bigint[]) EXCEPT SELECT unnest($5::bigint[]) ORDER BY 1)
        ) RETURNING record_id`, event, actor, target, previous, current).Scan(&id)
	if err != nil || collector == "" {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO iam_audit_deliveries(record_id,collector_key) VALUES($1,$2)`, id, collector)
	return err
}

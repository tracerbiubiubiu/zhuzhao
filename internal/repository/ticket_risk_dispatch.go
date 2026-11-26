package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// RequireGuardedCreateTx rejects bare transactions and guards for other pools.
// It proves connection ownership/order only; the admitting risk service must
// check its credential, dispatch permission, current owner and approved target.
func (r *TicketRepo) RequireGuardedCreateTx(ctx context.Context, pool *pgxpool.Pool, tx pgx.Tx) error {
	state, ok := ctx.Value(iamWriteKey{}).(*iamWriteState)
	if !ok || state == nil || tx == nil || state.tx == nil || state.pool != r.db || state.pool != pool || state.tx != tx {
		return errcode.ErrNoPermission
	}
	return nil
}

// BoundTicketDefinitionTx resolves the immutable rule on the supplied create
// connection, including the stored snapshot hash check.
func (r *TicketRepo) BoundTicketDefinitionTx(ctx context.Context, tx pgx.Tx, code string, version *int64) (*model.TicketDefinitionSnapshot, *model.TicketDefinitionRef, error) {
	return boundTicketDefinition(ctx, tx, code, version)
}

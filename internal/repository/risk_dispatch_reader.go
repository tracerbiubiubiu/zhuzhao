package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
)

// RiskDispatchReader is the query contract for the risk dispatch port.
type RiskDispatchReader interface {
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// RiskDispatchVisibleLinksSQL applies the same risk row scope used by list and
// detail. actorParam is a numbered PostgreSQL placeholder provided by the
// composition query, never request input.
func RiskDispatchVisibleLinksSQL(actorParam string) string {
	return `SELECT l.group_id, l.risk_id, l.ended_at
		FROM risk_ticket_links l
		JOIN risk_items ri ON ri.id = l.risk_id
		JOIN organizations go ON go.id = ri.org_id
		LEFT JOIN organizations ao ON ao.id = ri.affected_org_id
		WHERE ` + riskVisiblePredicate(actorParam)
}

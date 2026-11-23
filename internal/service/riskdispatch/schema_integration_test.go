//go:build integration

package riskdispatch

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestDispatchSchemaPreventsParallelTicketsForSameFindingAndOwner(t *testing.T) {
	db := dispatchTestDB
	ctx := context.Background()

	const createGroup = `
		INSERT INTO risk_dispatch_groups
			(finding_key, owner_user_id, management_org_id, target_org_id, type_code,
			 definition_version, approved_scope, group_key, ticket_id, ticket_ref_snapshot, operator_user_id)
		VALUES ($1, $2, 2, 3, 'synthetic_remediation', 1, 'synthetic', $3, $4, $5, 1)
	`
	var orgID int64
	require.NoError(t, db.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path) VALUES ('synthetic_risk_org', 'Synthetic', 'risk')
		RETURNING id
	`).Scan(&orgID))
	_, err := db.Exec(ctx, `
		INSERT INTO ticket_types (code, name) VALUES ('synthetic_remediation', 'Synthetic remediation')
	`)
	require.NoError(t, err)

	var tickets []int64
	for index := range 4 {
		var ticketID int64
		err := db.QueryRow(ctx, `
			INSERT INTO tickets (type_code, title, org_id, org_path, created_by)
			SELECT 'synthetic_remediation', 'synthetic risk dispatch', id, path, 1
			FROM organizations WHERE id = $1
			RETURNING id
		`, orgID).Scan(&ticketID)
		require.NoError(t, err)
		tickets = append(tickets, ticketID)
		require.Positive(t, tickets[index])
	}
	_, err = db.Exec(ctx, createGroup, "synthetic-finding", 101, strings.Repeat("1", 64), tickets[0], "SYN-01")
	require.NoError(t, err)
	_, err = db.Exec(ctx, createGroup, "synthetic-finding", 101, strings.Repeat("2", 64), tickets[1], "SYN-02")
	var conflict *pgconn.PgError
	require.True(t, errors.As(err, &conflict), "expected unique constraint, got %v", err)
	require.Equal(t, "23505", conflict.Code)
	require.Equal(t, "risk_dispatch_one_live_personal_group", conflict.ConstraintName)

	_, err = db.Exec(ctx, createGroup, "synthetic-finding", 102, strings.Repeat("3", 64), tickets[2], "SYN-03")
	require.NoError(t, err)
	_, err = db.Exec(ctx, createGroup, "other-finding", 101, strings.Repeat("4", 64), tickets[3], "SYN-04")
	require.NoError(t, err)

	_, err = db.Exec(ctx, "UPDATE risk_dispatch_groups SET ended_at = now() WHERE ticket_id = $1", tickets[0])
	require.NoError(t, err)
	_, err = db.Exec(ctx, createGroup, "synthetic-finding", 101, strings.Repeat("2", 64), tickets[1], "SYN-02")
	require.NoError(t, err)
}

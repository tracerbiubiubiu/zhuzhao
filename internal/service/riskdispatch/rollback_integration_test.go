//go:build integration

package riskdispatch

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

func TestRiskDownMigrationsRetainBusinessFacts(t *testing.T) {
	ctx := context.Background()
	db := dispatchTestDB
	f := setupDispatchFixture(t, db, "rollback-facts")
	risk := f.newRisk(t, db, "rollback-facts", "finding-rollback", "asset", f.ownerA, "high", testDeadline, "assessed")
	facts := map[string]bool{fmtKey(f.actorID, risk): true}
	d := newTestDispatcher(db, &stubAuthorizer{allowed: facts}, &stubFacts{visible: facts}, &stubTicketCreator{},
		stubTicketViews{all: true}, map[string]DefinitionRef{f.typeCode: {Version: 1, Hash: f.target.DefinitionHash}})
	op := applyPlan(t, d, f.actorID, buildPlanInput(f, []int64{risk}, map[int64]int64{risk: 1}), "op-migration-guard")
	result, err := d.Apply(ctx, f.actorID, op)
	require.NoError(t, err)
	require.Equal(t, "completed", result.Status)

	for _, migration := range []struct{ file, message string }{
		{"000090_risk_personal_dispatch.down.sql", "risk dispatch facts exist"},
		{"000089_risk_registration.down.sql", "risk registration facts exist"},
	} {
		t.Run(migration.file, func(t *testing.T) {
			sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", migration.file))
			require.NoError(t, err)
			tx, err := db.Begin(ctx)
			require.NoError(t, err)
			defer tx.Rollback(ctx)
			_, err = tx.Exec(ctx, string(sql))
			var pgErr *pgconn.PgError
			require.ErrorAs(t, err, &pgErr)
			require.Equal(t, "P0001", pgErr.Code)
			require.Contains(t, pgErr.Message, migration.message)
			require.NoError(t, tx.Rollback(ctx))

			receipt, err := d.ReadOperation(ctx, f.actorID, op.OperationID)
			require.NoError(t, err)
			require.Equal(t, result, receipt)
			require.Equal(t, int64(1), countTicketLinks(t, db, result.Groups[0].Ticket.ID))
		})
	}
}

//go:build integration

package repository_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// A pre-106 custom status could be absent from today's draft while still
// governing a pinned ticket. Test each source independently in a private schema.
func TestTicketHandlingMigrationReservesCancelled(t *testing.T) {
	ctx := context.Background()
	up, err := os.ReadFile("../../migrations/000106_ticket_handling.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("../../migrations/000106_ticket_handling.down.sql")
	require.NoError(t, err)
	for _, direction := range []string{"up", "down"} {
		for _, tc := range []struct {
			name, seed string
			blocked    bool
		}{
			{"draft state", `INSERT INTO ticket_types VALUES ('["open","cancelled","assigned","closed"]','{"open":["cancelled"],"cancelled":["open"]}','{}')`, true},
			{"draft edge destination", `INSERT INTO ticket_types VALUES ('[]','{"open":["cancelled"]}','{}')`, true},
			{"published historical source", `INSERT INTO ticket_type_definitions VALUES ('{"type":{"states":["open","assigned","closed","cancelled"],"transitions":{"cancelled":["open"]}}}')`, true},
			{"published creator withdrawal", `INSERT INTO ticket_type_definitions VALUES ('{"type":{"states":["open","assigned","closed","cancelled"],"transitions":{"open":["assigned","closed","cancelled"]},"transition_meta":{"open>cancelled":{"roles":["creator"]}}}}')`, true},
			{"instance", `INSERT INTO tickets(status,assigned_to) VALUES ('cancelled',NULL)`, true},
			{"ordinary label", `INSERT INTO ticket_types VALUES ('["open","closed"]','{"open":["closed"]}','{"open>closed":{"label":"cancelled"}}')`, false},
		} {
			t.Run(direction+"/"+tc.name, func(t *testing.T) {
				tx, err := testPool.Begin(ctx)
				require.NoError(t, err)
				defer tx.Rollback(ctx)
				_, err = tx.Exec(ctx, `CREATE SCHEMA handling_preflight; SET LOCAL search_path=handling_preflight,pg_catalog;
    CREATE TABLE ticket_types(states jsonb,transitions jsonb,transition_meta jsonb);
    CREATE TABLE ticket_type_definitions(snapshot jsonb);
    CREATE TABLE tickets(status text,assigned_to bigint);
    CREATE TABLE menus (LIKE public.menus); CREATE TABLE menu_apis (LIKE public.menu_apis);
    CREATE TABLE roles (LIKE public.roles); CREATE TABLE role_menus (LIKE public.role_menus);
    CREATE TABLE casbin_rule (LIKE public.casbin_rule)`)
				require.NoError(t, err)
				migration, reason := up, "legacy cancelled"
				if direction == "down" {
					_, err = tx.Exec(ctx, string(up))
					require.NoError(t, err)
					migration, reason = down, "retain cancelled"
				}
				_, err = tx.Exec(ctx, tc.seed)
				require.NoError(t, err)
				_, err = tx.Exec(ctx, "SAVEPOINT migration_start")
				require.NoError(t, err)
				_, err = tx.Exec(ctx, string(migration))
				if tc.blocked {
					require.ErrorContains(t, err, reason)
					_, err = tx.Exec(ctx, "ROLLBACK TO SAVEPOINT migration_start")
					require.NoError(t, err)
				} else {
					require.NoError(t, err)
				}
				var installed, eligibility bool
				require.NoError(t, tx.QueryRow(ctx, `SELECT to_regclass('handling_preflight.ticket_handling_receipts') IS NOT NULL, EXISTS(SELECT 1 FROM pg_attribute WHERE attrelid='tickets'::regclass AND attname='withdrawal_eligible' AND NOT attisdropped)`).Scan(&installed, &eligibility))
				expectInstalled := direction == "up" && !tc.blocked || direction == "down" && tc.blocked
				require.Equal(t, expectInstalled, installed)
				require.Equal(t, expectInstalled, eligibility)
			})
		}
	}
}

// A rollback started before the writer commits must still see its receipt.
// Keep the DDL in an explicit test transaction; release every acquired lock.
func TestTicketHandlingRollbackWaitsForReceipt(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	down, err := os.ReadFile("../../migrations/000106_ticket_handling.down.sql")
	require.NoError(t, err)
	writer, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer writer.Rollback(ctx)
	const key = "handling-migration-concurrent"
	_, err = writer.Exec(ctx, `INSERT INTO ticket_handling_receipts(actor_id,operation_key,request_hash,ticket_id,result_status,result_revision)
 VALUES(1,$1,decode(repeat('01',32),'hex'),1,'assigned',2)`, key)
	require.NoError(t, err)
	// After the writer commits, this synthetic receipt is the only persistent fixture.
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), "DELETE FROM ticket_handling_receipts WHERE actor_id=1 AND operation_key=$1", key)
		require.NoError(t, err)
	})
	migration, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer migration.Rollback(ctx)
	var pid int32
	require.NoError(t, migration.QueryRow(ctx, "SELECT pg_backend_pid()").Scan(&pid))
	done := make(chan error, 1)
	go func() { _, err := migration.Exec(ctx, string(down)); done <- err }()
	require.Eventually(t, func() bool {
		var blocked bool
		err := testPool.QueryRow(ctx, "SELECT cardinality(pg_blocking_pids($1))>0", pid).Scan(&blocked)
		return err == nil && blocked
	}, 5*time.Second, 10*time.Millisecond)
	require.NoError(t, writer.Commit(ctx))
	require.ErrorContains(t, <-done, "retain ticket handling receipts before rollback")
}

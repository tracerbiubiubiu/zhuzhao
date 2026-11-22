//go:build integration

package service_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const d01MigrationUp = "../../migrations/000084_ticket_definitions.up.sql"

func applyD01TestMigration(t *testing.T, db *pgxpool.Pool, path string) {
	t.Helper()
	sql, err := os.ReadFile(path)
	require.NoError(t, err)
	_, err = db.Exec(context.Background(), string(sql))
	require.NoError(t, err, path)
}

func checkD01CreateAfterUpgrade(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	// Use a fresh database in this test package's disposable PG container.
	dbName := "d01_upgrade_" + uniqueSuffix()
	dbIdent := pgx.Identifier{dbName}.Sanitize()
	_, err := testPool.Exec(ctx, "CREATE DATABASE "+dbIdent)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(ctx, "DROP DATABASE "+dbIdent)
		require.NoError(t, err)
	})
	cfg := testPool.Config()
	cfg.ConnConfig.Database = dbName
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	paths, err := filepath.Glob("../../migrations/*.up.sql")
	require.NoError(t, err)
	for _, path := range paths {
		if path == d01MigrationUp {
			break
		}
		applyD01TestMigration(t, db, path)
	}

	f := newCreateC04InDB(t, db, nil)
	_, err = db.Exec(ctx, `UPDATE ticket_types SET version=3, transition_meta='{"open":{"sla_due_hours":4}}' WHERE code=$1`, f.code)
	require.NoError(t, err)
	var oldID int64
	require.NoError(t, db.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path)
		VALUES($1::text,'original ticket','closed',$2,$3,$1::text::ltree) RETURNING id`, f.code, f.actor, f.org).Scan(&oldID))
	// The v83 cohort is frozen by 84. Apply the later schema before running
	// the current application, which also reads the D02 revision.
	for _, path := range paths {
		if path >= d01MigrationUp {
			applyD01TestMigration(t, db, path)
		}
	}
	_, err = db.Exec(ctx, `UPDATE ticket_types SET version=4, transition_meta='{"open":{"sla_due_hours":9}}' WHERE code=$1`, f.code)
	require.NoError(t, err)
	_, err = db.Exec(ctx, `UPDATE ticket_type_fields SET field_meta='{}', required=true WHERE type_code=$1 AND field_key='result'`, f.code)
	require.NoError(t, err)

	w := f.request(ctx, "allow")
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var priority, events int
	var description, data, key string
	var version, draftRevision int64
	var statusChangedAt, dueAt time.Time
	require.NoError(t, db.QueryRow(ctx, `SELECT t.priority,t.description,t.custom_data,t.human_key,
		t.definition_version,d.source_draft_revision,t.status_changed_at,t.sla_due_at,
		(SELECT count(*) FROM ticket_events WHERE ticket_id=t.id AND action='created')
		FROM tickets t JOIN ticket_type_definitions d USING(type_code,definition_version)
		WHERE t.type_code=$1 AND t.id<>$2`, f.code, oldID).
		Scan(&priority, &description, &data, &key, &version, &draftRevision, &statusChangedAt, &dueAt, &events))
	require.Equal(t, 2, priority)
	require.Equal(t, "template description", description)
	require.JSONEq(t, `{"source":"synthetic","result":"prepared"}`, data)
	require.NotEmpty(t, key)
	require.EqualValues(t, 1, version)
	require.EqualValues(t, 3, draftRevision)
	require.Equal(t, 4*time.Hour, dueAt.Sub(statusChangedAt))
	require.Equal(t, 1, events)
	require.Len(t, f.trace.pids, 1, "fresh authorization and persistence share one connection")
	var oldVersion *int64
	var oldStatus string
	require.NoError(t, db.QueryRow(ctx, `SELECT definition_version,status FROM tickets WHERE id=$1`, oldID).Scan(&oldVersion, &oldStatus))
	require.Nil(t, oldVersion)
	require.Equal(t, "closed", oldStatus)

	// The seeded incident draft has not changed since 83. A later explicit
	// publication of identical content must reuse its migration baseline.
	draft, err := repository.NewTicketRepo(db).GetTicketType(ctx, "incident")
	require.NoError(t, err)
	baseline, changed, err := f.svc.PublishTicketType(ctx, draft, f.actor)
	require.NoError(t, err)
	require.False(t, changed)
	require.Nil(t, baseline.PublishedBy, "the cutover has no publishing user")
	require.Regexp(t, `^[a-f0-9]{64}$`, baseline.DefinitionHash)
}

// Fixtures created after 84 explicitly publish once their draft is complete.
func publishCreateD01(t *testing.T, f *createC04Fixture) {
	t.Helper()
	ctx := context.Background()
	repo := repository.NewTicketRepo(testPool)
	old, err := repo.ListTicketTypeFieldsAll(ctx, f.code)
	require.NoError(t, err)
	fields := make([]model.TicketTypeField, 0, len(old))
	for _, field := range old {
		fields = append(fields, *field)
	}
	require.NoError(t, repo.ReplaceTypeFields(ctx, f.code, fields, nil))
	draft, err := repo.GetTicketType(ctx, f.code)
	require.NoError(t, err)
	_, changed, err := f.svc.PublishTicketType(ctx, draft, f.actor)
	require.NoError(t, err)
	require.True(t, changed)
}

func TestD01PublishedCreateRetainsCurrentIAMGuard(t *testing.T) {
	for _, change := range []string{"allow", "account", "role", "policy", "membership"} {
		t.Run(change, func(t *testing.T) {
			f := newCreateC04(t, func(f *createC04Fixture) {
				require.NoError(t, repository.WithIAMGuard(context.Background(), testPool, func(ctx context.Context) error {
					q := repository.IAMDB(ctx, testPool)
					var err error
					switch change {
					case "account":
						_, err = q.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, f.actor)
					case "role":
						_, err = q.Exec(ctx, `UPDATE roles SET status=0 WHERE id=$1`, f.role)
					case "policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1`, "role::"+f.code)
					case "membership":
						_, err = q.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1`, f.actor)
					}
					return err
				}))
			})
			publishCreateD01(t, f)
			w := f.request(context.Background(), "")
			if change == "allow" {
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				f.counts(t, 1)
				var version int64
				require.NoError(t, testPool.QueryRow(context.Background(), `SELECT definition_version FROM tickets WHERE type_code=$1`, f.code).Scan(&version))
				require.EqualValues(t, 1, version)
			} else {
				require.NotEqual(t, http.StatusOK, w.Code, w.Body.String())
				f.counts(t, 0)
			}
		})
	}
}

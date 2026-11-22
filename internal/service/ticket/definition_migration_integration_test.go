//go:build integration

package ticket

import (
	"context"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestD01MigrationExpansionRoundTrip(t *testing.T) {
	ctx := context.Background()
	up, err := os.ReadFile("../../../migrations/000084_ticket_definitions.up.sql")
	require.NoError(t, err)
	down, err := os.ReadFile("../../../migrations/000084_ticket_definitions.down.sql")
	require.NoError(t, err)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	schema := "d01_migration_" + uniqueSuffix()
	_, err = tx.Exec(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`;
 CREATE TABLE users(id BIGINT PRIMARY KEY);
 CREATE TABLE ticket_types(LIKE public.ticket_types INCLUDING DEFAULTS INCLUDING INDEXES);
 ALTER TABLE ticket_types DROP COLUMN current_published_version;
 CREATE TABLE tickets(id BIGINT PRIMARY KEY,type_code VARCHAR(50),status TEXT);
 CREATE TABLE ticket_type_fields(LIKE public.ticket_type_fields INCLUDING DEFAULTS);
 INSERT INTO ticket_types(id,code,name) VALUES(1,'synthetic_old','Old'),(3,'malformed','Malformed');
 INSERT INTO tickets VALUES(1,'synthetic_old','open');
 INSERT INTO ticket_type_fields(id,type_code,field_key,field_label,field_type,required)
  VALUES(1,'synthetic_old','plan','Plan','input',true),(2,'malformed','plan','Plan','input',NULL);`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(up))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO ticket_types(id,code,name) VALUES(2,'new_draft','New draft')`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(up))
	require.NoError(t, err, "the migration can be replayed")
	var pointer *int64
	require.NoError(t, tx.QueryRow(ctx, `SELECT current_published_version FROM ticket_types WHERE code='new_draft'`).Scan(&pointer))
	require.Nil(t, pointer, "replay must leave a later draft unpublished")
	require.NoError(t, tx.QueryRow(ctx, `SELECT current_published_version FROM ticket_types WHERE code='malformed'`).Scan(&pointer))
	require.Nil(t, pointer, "a NULL legacy requirement must not turn into an optional field")
	var count int
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM ticket_type_definitions WHERE type_code='synthetic_old' AND published_by IS NULL`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE definition_version IS NULL`).Scan(&count))
	require.Equal(t, 1, count)
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM ticket_field_identities`).Scan(&count))
	require.Equal(t, 2, count)
	_, err = tx.Exec(ctx, string(down))
	require.NoError(t, err, "unused, unchanged cutover baselines allow rollback")
	require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE id=1 AND status='open'`).Scan(&count))
	require.Equal(t, 1, count)

	_, err = tx.Exec(ctx, string(up))
	require.NoError(t, err)
	for _, tc := range []struct{ name, change string }{
		{"bound ticket", `UPDATE tickets SET definition_version=1 WHERE id=1`},
		{"changed draft", `UPDATE ticket_types SET version=2 WHERE code='synthetic_old'`},
		{"explicit publication", `INSERT INTO users VALUES(1);
 INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,schema_format_version,
  snapshot,source_draft_revision,published_by) VALUES('synthetic_old',2,'synthetic',1,'{}',1,1)`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := tx.Exec(ctx, `SAVEPOINT before_change;`+tc.change+`; SAVEPOINT before_down;`)
			require.NoError(t, err)
			_, err = tx.Exec(ctx, string(down))
			require.ErrorContains(t, err, "retain definitions and bindings before rollback")
			_, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_down`)
			require.NoError(t, err)
			require.NoError(t, tx.QueryRow(ctx, `SELECT count(*) FROM ticket_types WHERE current_published_version=1`).Scan(&count))
			require.Equal(t, 2, count, "a rejected down preserves current pointers")
			_, err = tx.Exec(ctx, `ROLLBACK TO SAVEPOINT before_change; RELEASE SAVEPOINT before_change`)
			require.NoError(t, err)
		})
	}
}

func TestD01MigrationCutoverRulesMustBeUsable(t *testing.T) {
	ctx := context.Background()
	up, err := os.ReadFile("../../../migrations/000084_ticket_definitions.up.sql")
	require.NoError(t, err)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	schema := "d01_cutover_" + uniqueSuffix()
	_, err = tx.Exec(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`;
 CREATE TABLE users(id BIGINT PRIMARY KEY);
 CREATE TABLE ticket_types(LIKE public.ticket_types INCLUDING DEFAULTS INCLUDING INDEXES);
 ALTER TABLE ticket_types DROP COLUMN current_published_version;
 CREATE TABLE tickets(id BIGINT PRIMARY KEY,type_code VARCHAR(50),status TEXT);
 CREATE TABLE ticket_type_fields(LIKE public.ticket_type_fields INCLUDING DEFAULTS);`)
	require.NoError(t, err)

	const states = `["open","assigned","closed"]`
	const transitions = `{"open":["assigned","closed"],"assigned":["closed"]}`
	const validMeta = `{"open":{"timeout_minutes":30},"open>assigned":{"roles":["assignee"]}}`
	cases := []struct {
		code, states, transitions, meta, fieldType string
		valid                                      bool
	}{
		{"valid", states, transitions, validMeta, "file", true},
		{"legacy_no_meta", states, transitions, `null`, "", true},
		{"non_array_states", `{"open":true}`, transitions, validMeta, "", false},
		{"missing_assigned", `["open","closed"]`, `{"open":["closed"]}`, `{}`, "", false},
		{"duplicated_state", `["open","assigned","closed","closed"]`, transitions, validMeta, "", false},
		{"whitespace_state", `["open","assigned","closed"," invalid"]`, transitions, validMeta, "", false},
		{"bad_edge_source", states, `{"missing":["closed"]}`, `{}`, "", false},
		{"bad_edge_target", states, `{"open":["missing"]}`, `{}`, "", false},
		{"bad_edge_value", states, `{"open":42}`, `{}`, "", false},
		{"bad_transition_shape", states, `[]`, `{}`, "", false},
		{"orphan_edge_meta", states, transitions, `{"closed>assigned":{}}`, "", false},
		{"unknown_state_meta", states, transitions, `{"obsolete":{}}`, "", false},
		{"non_object_meta", states, transitions, `[{}]`, "", false},
		{"non_object_meta_segment", states, transitions, `{"open>assigned":[]}`, "", false},
		{"unsupported_field", states, transitions, validMeta, "unsupported", false},
	}
	for i, tc := range cases {
		_, err = tx.Exec(ctx, `INSERT INTO ticket_types(id,code,name,states,transitions,transition_meta)
 VALUES($1,$2,'Synthetic cutover',$3::jsonb,$4::jsonb,$5::jsonb)`, i+1, tc.code, tc.states, tc.transitions, tc.meta)
		require.NoError(t, err)
		if tc.fieldType != "" {
			_, err = tx.Exec(ctx, `INSERT INTO ticket_type_fields(id,type_code,field_key,field_label,field_type,required)
 VALUES($1,$2,'proof','Proof',$3,TRUE)`, i+1, tc.code, tc.fieldType)
			require.NoError(t, err)
		}
	}
	_, err = tx.Exec(ctx, string(up))
	require.NoError(t, err, "bad legacy rows must not abort the migration for other types")
	for _, tc := range cases {
		t.Run(tc.code, func(t *testing.T) {
			var version *int64
			var snapshots int
			require.NoError(t, tx.QueryRow(ctx, `SELECT current_published_version,
 (SELECT count(*) FROM ticket_type_definitions WHERE type_code = t.code)
 FROM ticket_types t WHERE code=$1`, tc.code).Scan(&version, &snapshots))
			if tc.valid {
				require.NotNil(t, version)
				require.EqualValues(t, 1, *version)
				require.Equal(t, 1, snapshots)
			} else {
				require.Nil(t, version)
				require.Zero(t, snapshots)
			}
		})
	}
}

func TestD01MigrationKeepsPublisherHistoryIndependentFromUsers(t *testing.T) {
	ctx := context.Background()
	up, err := os.ReadFile("../../../migrations/000084_ticket_definitions.up.sql")
	require.NoError(t, err)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	schema := "d01_publisher_" + uniqueSuffix()
	_, err = tx.Exec(ctx, `CREATE SCHEMA `+schema+`; SET LOCAL search_path TO `+schema+`;
 CREATE TABLE users(id BIGINT PRIMARY KEY);
 CREATE TABLE ticket_types(LIKE public.ticket_types INCLUDING DEFAULTS INCLUDING INDEXES);
 ALTER TABLE ticket_types DROP COLUMN current_published_version;
 CREATE TABLE tickets(id BIGINT PRIMARY KEY,type_code VARCHAR(50),status TEXT);
 CREATE TABLE ticket_type_fields(LIKE public.ticket_type_fields INCLUDING DEFAULTS);
 INSERT INTO users VALUES(17);
 INSERT INTO ticket_types(id,code,name) VALUES(1,'synthetic_publisher','Synthetic publisher');`)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(up))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `INSERT INTO ticket_type_definitions
   (type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by)
 SELECT type_code,2,definition_hash,schema_format_version,snapshot,source_draft_revision,17
 FROM ticket_type_definitions WHERE type_code='synthetic_publisher' AND definition_version=1;
 TRUNCATE users CASCADE;`)
	require.NoError(t, err)
	var publisher int64
	require.NoError(t, tx.QueryRow(ctx, `SELECT published_by FROM ticket_type_definitions
 WHERE type_code='synthetic_publisher' AND definition_version=2`).Scan(&publisher))
	require.EqualValues(t, 17, publisher)
}

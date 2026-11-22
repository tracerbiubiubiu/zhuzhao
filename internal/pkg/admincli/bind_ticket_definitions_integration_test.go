//go:build integration

package admincli_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/admincli"
)

func bindTestMigrationSQL(t *testing.T, filename string) string {
	t.Helper()
	sql, err := os.ReadFile(filepath.Join("..", "..", "..", "migrations", filename))
	require.NoError(t, err)
	return string(sql)
}

func TestBindTicketDefinitions(t *testing.T) {
	ctx := context.Background()
	down := bindTestMigrationSQL(t, "000086_ticket_legacy_definition_bindings.down.sql")
	t.Run("empty 086 migration can be rolled back and reapplied", func(t *testing.T) {
		tx, err := testPool.Begin(ctx)
		require.NoError(t, err)
		defer func() { _ = tx.Rollback(ctx) }()
		_, err = tx.Exec(ctx, `CREATE SCHEMA legacy_bind_migration_test;
			SET LOCAL search_path TO legacy_bind_migration_test, public;`)
		require.NoError(t, err)
		up := bindTestMigrationSQL(t, "000086_ticket_legacy_definition_bindings.up.sql")
		for _, migration := range []string{up, down, up} {
			_, err = tx.Exec(ctx, migration)
			require.NoError(t, err)
		}
	})

	actorID := createSuperadminUser(t, 1)
	suffix := strconv.FormatInt(actorID, 10)
	orgCode := "legacy_bind_org_" + suffix
	typeCode := "legacy_bind_target_" + suffix
	otherType := "legacy_bind_other_" + suffix
	var orgID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, path)
		VALUES ($1, 'Legacy bind test', $2::text::ltree) RETURNING id`, orgCode, orgCode,
	).Scan(&orgID))
	// Synthetic evidence captured before the cutover. The manifest digest is
	// computed from this artifact independently of the stored definition.
	const oldConfigurationTemplate = `{
		"type": {
			"id": "0", "code": %[1]q, "name": "Legacy bind test", "description": "Approved legacy configuration",
			"states": ["open", "assigned", "in_progress", "pending_verify", "closed", "rejected", "hold_forever"],
			"transitions": {
				"open": ["assigned", "closed"], "assigned": ["in_progress", "open"],
				"in_progress": ["pending_verify", "rejected", "closed"],
				"pending_verify": ["closed", "in_progress"], "closed": ["open"], "rejected": ["open"]
			},
			"transition_meta": {}, "default_sla_hours": 24, "has_custom_fields": true,
			"is_active": false, "version": 0, "created_at": "0001-01-01T00:00:00Z",
			"current_published_version": null
		},
		"fields": [
			{"id":"0","field_id":"10","type_code":%[1]q,"field_key":"original","field_label":"Original evidence",
			 "field_type":"input","required":true,"sort_order":0}
		]
	}`
	for _, code := range []string{typeCode, otherType} {
		_, err := testPool.Exec(ctx, `
			INSERT INTO ticket_types (code, name, description)
			VALUES ($1, 'Legacy bind test', 'Unpublished draft')`, code)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `
			INSERT INTO ticket_type_definitions
				(type_code, definition_version, definition_hash, schema_format_version, snapshot, source_draft_revision)
			VALUES ($1, 1, ticket_definition_snapshot_hash($2::jsonb), 1, $2::jsonb, 1)`,
			code, fmt.Sprintf(oldConfigurationTemplate, code))
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `
			INSERT INTO ticket_field_identities (type_code, field_id, field_key)
			VALUES ($1, 10, 'original')`, code)
		require.NoError(t, err)
	}
	_, err := testPool.Exec(ctx, `
		WITH rules AS (
			SELECT jsonb_set(snapshot, '{type,description}', '"Current release"'::jsonb) AS snapshot
			FROM ticket_type_definitions WHERE type_code = $1 AND definition_version = 1
		)
		INSERT INTO ticket_type_definitions
			(type_code, definition_version, definition_hash, schema_format_version, snapshot, source_draft_revision, published_by)
		SELECT $1, 2, ticket_definition_snapshot_hash(snapshot), 1, snapshot, 1, $2 FROM rules`, typeCode, actorID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE ticket_types SET current_published_version = 2 WHERE code = $1`, typeCode)
	require.NoError(t, err)

	oldConfiguration := fmt.Sprintf(oldConfigurationTemplate, typeCode)
	definitionHash := queryString(t, `SELECT ticket_definition_snapshot_hash($1::jsonb)`, oldConfiguration)
	currentHash := queryString(t, `
		SELECT definition_hash FROM ticket_type_definitions WHERE type_code = $1 AND definition_version = 2`, typeCode)
	require.NotEqual(t, definitionHash, currentHash)
	_, err = testPool.Exec(ctx, `
		INSERT INTO ticket_type_definitions
			(type_code, definition_version, definition_hash, schema_format_version, snapshot, source_draft_revision, published_by)
		SELECT type_code, 3, $2, 1, snapshot, source_draft_revision, published_by
		FROM ticket_type_definitions WHERE type_code = $1 AND definition_version = 2`, typeCode, definitionHash)
	require.NoError(t, err)
	var incompleteHash string
	require.NoError(t, testPool.QueryRow(ctx, `
		WITH rules AS (
			SELECT snapshot - 'fields' AS snapshot
			FROM ticket_type_definitions WHERE type_code = $1 AND definition_version = 1
		)
		INSERT INTO ticket_type_definitions
			(type_code, definition_version, definition_hash, schema_format_version, snapshot, source_draft_revision, published_by)
		SELECT $1, 4, ticket_definition_snapshot_hash(snapshot), 1, snapshot, 1, $2 FROM rules
		RETURNING definition_hash`, typeCode, actorID).Scan(&incompleteHash))
	var unsupportedFieldHash string
	require.NoError(t, testPool.QueryRow(ctx, `
		WITH rules AS (
			SELECT jsonb_set(snapshot, '{fields,0,field_type}', '"unsupported"'::jsonb) AS snapshot
			FROM ticket_type_definitions WHERE type_code = $1 AND definition_version = 1
		)
		INSERT INTO ticket_type_definitions
			(type_code, definition_version, definition_hash, schema_format_version, snapshot, source_draft_revision, published_by)
		SELECT $1, 5, ticket_definition_snapshot_hash(snapshot), 1, snapshot, 1, $2 FROM rules
		RETURNING definition_hash`, typeCode, actorID).Scan(&unsupportedFieldHash))
	originalCreatedAt := time.Date(2026, 10, 1, 2, 3, 4, 567890000, time.UTC)
	insertTicket := func(t *testing.T, key, code, status string) int64 {
		t.Helper()
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, `
			INSERT INTO tickets (
				human_key, title, description, type_code, status, priority, created_by, org_id, org_path,
				assigned_to, created_at, updated_at, status_changed_at, sla_due_at, custom_data)
			VALUES ($1, 'Synthetic original title', 'Original evidence reference', $2, $3, 2, $4, $5,
				$7::text::ltree, $4, $6::timestamptz, $6 + interval '1 hour', $6 + interval '1 hour',
				$6 + interval '4 hours', '{"original":"value"}')
			RETURNING id`, key+"-"+suffix, code, status, actorID, orgID, originalCreatedAt, orgCode).Scan(&id))
		return id
	}
	selected := insertTicket(t, "T-100", typeCode, "in_progress")
	unselected := insertTicket(t, "T-101", typeCode, "in_progress")
	outsideType := insertTicket(t, "T-102", otherType, "in_progress")
	closed := insertTicket(t, "T-103", typeCode, "closed")
	rejected := insertTicket(t, "T-104", typeCode, "rejected")
	terminal := insertTicket(t, "T-105", typeCode, "hold_forever")

	_, err = testPool.Exec(ctx, `
		INSERT INTO ticket_events (ticket_id, user_id, action, from_value, to_value)
		VALUES ($1, $2, 'status_changed', 'assigned', 'in_progress')`, selected, actorID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `
		INSERT INTO ticket_comments (ticket_id, user_id, content)
		VALUES ($1, $2, 'Original comment')`, selected, actorID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `
		WITH file AS (
			INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, size_bytes, etag, created_by)
			VALUES ('fixture', 'tickets/' || $1::bigint::text || '/original.txt', 'ticket_attachment', 'confirmed', 'original.txt',
				'text/plain', 10, 'fixture-etag', $2) RETURNING id
		)
		INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by)
		SELECT $1, id, $2 FROM file`, selected, actorID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `
		INSERT INTO ticket_reminders (ticket_id, state, reminder_count, last_reminder_at)
		VALUES ($1, 'in_progress', 2, $2)`, selected, originalCreatedAt.Add(2*time.Hour))
	require.NoError(t, err)

	businessState := func(t *testing.T) string {
		t.Helper()
		return queryString(t, `
			SELECT jsonb_build_object(
				'tickets', (SELECT jsonb_agg(to_jsonb(t) - 'definition_version' ORDER BY id)
					FROM tickets t WHERE org_id = $1),
				'types', (SELECT jsonb_agg(to_jsonb(t) ORDER BY code)
					FROM ticket_types t WHERE code IN ($3, $4)),
				'definitions', (SELECT jsonb_agg(to_jsonb(d) ORDER BY type_code, definition_version)
					FROM ticket_type_definitions d WHERE type_code IN ($3, $4)),
				'events', (SELECT jsonb_agg(to_jsonb(e) ORDER BY e.id) FROM ticket_events e
					JOIN tickets t ON t.id = e.ticket_id WHERE t.org_id = $1),
				'comments', (SELECT jsonb_agg(to_jsonb(c) ORDER BY id) FROM ticket_comments c WHERE c.ticket_id = $2),
				'reminders', (SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM ticket_reminders r WHERE r.ticket_id = $2),
				'attachments', (SELECT jsonb_agg(to_jsonb(a) ORDER BY id) FROM ticket_attachments a WHERE a.ticket_id = $2),
				'files', (SELECT jsonb_agg(to_jsonb(f) ORDER BY f.id) FROM ticket_attachments a
					JOIN file_objects f ON f.id = a.file_object_id WHERE a.ticket_id = $2)
			)::text`, orgID, selected, typeCode, otherType)
	}
	baseline := businessState(t)
	receiptBaseline := queryInt(t, `SELECT count(*) FROM ticket_legacy_definition_bindings`)
	requireUnchanged := func(t *testing.T) {
		t.Helper()
		require.Equal(t, receiptBaseline, queryInt(t, `SELECT count(*) FROM ticket_legacy_definition_bindings`))
		require.Zero(t, queryInt(t, `SELECT count(*) FROM tickets WHERE org_id = $1 AND definition_version IS NOT NULL`, orgID))
		require.Equal(t, baseline, businessState(t))
	}
	manifest := func(id int64, status string) map[string]string {
		return map[string]string{
			"ticket_id":                     strconv.FormatInt(id, 10),
			"type_code":                     typeCode,
			"original_created_by":           strconv.FormatInt(actorID, 10),
			"original_created_at":           originalCreatedAt.Format(time.RFC3339Nano),
			"expected_status":               status,
			"definition_version":            "1",
			"definition_hash":               definitionHash,
			"configuration_evidence_sha256": bindEvidenceDigest(oldConfiguration),
			"mapping_evidence_sha256":       bindEvidenceDigest("synthetic ticket mapping"),
			"approval_sha256":               bindEvidenceDigest("synthetic approval"),
			"ticket_evidence_sha256":        bindEvidenceDigest("synthetic original ticket record"),
		}
	}
	run := func(t *testing.T, fields map[string]string) (string, error) {
		t.Helper()
		input, err := json.Marshal(fields)
		require.NoError(t, err)
		var output bytes.Buffer
		err = admincli.RunBindTicketDefinitions(ctx, testPool.Config().ConnString(), bytes.NewReader(input), &output)
		return output.String(), err
	}

	t.Run("rejects unknown, mismatched, and terminal tickets without writes", func(t *testing.T) {
		for _, tc := range []struct {
			name   string
			id     int64
			status string
			field  string
			value  string
			reason string
		}{
			{"absent approval", selected, "in_progress", "approval_sha256", "", "批准及原票的 SHA-256 摘要"},
			{"unknown ticket", 9223372036854775807, "in_progress", "", "", "未找到指定工单"},
			{"unknown definition", selected, "in_progress", "definition_version", "9999", "指定旧定义不存在"},
			{"other version", selected, "in_progress", "definition_version", "2", "完整快照摘要不匹配"},
			{"stored hash does not match contents", selected, "in_progress", "definition_version", "3", "完整快照摘要不匹配"},
			{"incorrect hash", selected, "in_progress", "definition_hash", bindEvidenceDigest("incorrect definition"), "完整快照摘要不匹配"},
			{"wrong type", outsideType, "in_progress", "", "", "原票事实与清单不匹配"},
			{"wrong creator", selected, "in_progress", "original_created_by", strconv.FormatInt(actorID+1, 10), "原票事实与清单不匹配"},
			{"wrong creation time", selected, "in_progress", "original_created_at", originalCreatedAt.Add(time.Microsecond).Format(time.RFC3339Nano), "原票事实与清单不匹配"},
			{"wrong status", selected, "assigned", "", "", "当前工单状态与清单不匹配"},
			{"closed with reopen edge", closed, "closed", "", "", "终态工单不能补绑"},
			{"rejected with reopen edge", rejected, "rejected", "", "", "终态工单不能补绑"},
			{"custom state with no outgoing edge", terminal, "hold_forever", "", "", "终态工单不能补绑"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				fields := manifest(tc.id, tc.status)
				if tc.field != "" {
					fields[tc.field] = tc.value
				}
				output, runErr := run(t, fields)
				require.ErrorContains(t, runErr, tc.reason)
				require.Empty(t, output)
				requireUnchanged(t)
			})
		}
		t.Run("incomplete snapshot has a self-consistent hash", func(t *testing.T) {
			fields := manifest(selected, "in_progress")
			fields["definition_version"] = "4"
			fields["definition_hash"] = incompleteHash
			output, err := run(t, fields)
			require.ErrorContains(t, err, "旧定义快照不完整")
			require.Empty(t, output)
			requireUnchanged(t)
		})
		t.Run("unsupported field in hash-matched definition", func(t *testing.T) {
			fields := manifest(selected, "in_progress")
			fields["definition_version"] = "5"
			fields["definition_hash"] = unsupportedFieldHash
			output, err := run(t, fields)
			require.ErrorContains(t, err, "旧定义快照不完整")
			require.Empty(t, output)
			requireUnchanged(t)
		})
	})

	t.Run("rolls back an unexpected ticket-row mutation from a trigger", func(t *testing.T) {
		_, err := testPool.Exec(ctx, `
			CREATE FUNCTION legacy_bind_test_move_updated_at() RETURNS trigger LANGUAGE plpgsql AS $$
			BEGIN
				UPDATE tickets SET updated_at = OLD.updated_at + interval '1 day' WHERE id = NEW.id;
				RETURN NULL;
			END;
			$$;
			CREATE TRIGGER legacy_bind_test_move_updated_at
				AFTER UPDATE OF definition_version ON tickets
				FOR EACH ROW EXECUTE FUNCTION legacy_bind_test_move_updated_at();`)
		require.NoError(t, err)
		t.Cleanup(func() {
			_, err := testPool.Exec(ctx, `
				DROP TRIGGER legacy_bind_test_move_updated_at ON tickets;
				DROP FUNCTION legacy_bind_test_move_updated_at();`)
			require.NoError(t, err)
		})
		output, err := run(t, manifest(selected, "in_progress"))
		require.ErrorContains(t, err, "补绑触发了工单业务字段变化")
		require.Empty(t, output)
		requireUnchanged(t)
	})

	fields := manifest(selected, "in_progress")
	require.EqualValues(t, 1, queryInt(t, `SELECT revision FROM tickets WHERE id = $1`, selected))
	input, err := json.Marshal(fields)
	require.NoError(t, err)
	err = admincli.RunBindTicketDefinitions(ctx, testPool.Config().ConnString(), bytes.NewReader(input), closedBindOutput{})
	require.ErrorContains(t, err, "回执输出失败")
	require.EqualValues(t, 1, queryInt(t, `SELECT definition_version FROM tickets WHERE id = $1`, selected))
	require.EqualValues(t, 1, queryInt(t, `SELECT count(*) FROM ticket_legacy_definition_bindings WHERE ticket_id = $1`, selected))
	first, err := run(t, fields)
	require.NoError(t, err)
	var receipt map[string]string
	require.NoError(t, json.Unmarshal([]byte(first), &receipt))
	boundAt, err := time.Parse(time.RFC3339Nano, receipt["bound_at"])
	require.NoError(t, err)
	require.False(t, boundAt.IsZero())
	delete(receipt, "bound_at")
	require.Equal(t, fields, receipt)
	require.EqualValues(t, 1, queryInt(t, `SELECT definition_version FROM tickets WHERE id = $1`, selected))
	require.EqualValues(t, 1, queryInt(t, `SELECT revision FROM tickets WHERE id = $1`, selected))
	require.Zero(t, queryInt(t, `
		SELECT count(*) FROM tickets WHERE id IN ($1, $2, $3, $4, $5) AND definition_version IS NOT NULL`,
		unselected, outsideType, closed, rejected, terminal))
	require.EqualValues(t, 1, queryInt(t, `SELECT count(*) FROM ticket_legacy_definition_bindings WHERE ticket_id = $1`, selected))
	require.Equal(t, baseline, businessState(t), "binding may only add the definition reference and its receipt")

	replay, err := run(t, fields)
	require.NoError(t, err)
	require.JSONEq(t, first, replay)
	_, err = testPool.Exec(ctx, `UPDATE tickets SET status = 'closed', updated_at = now() WHERE id = $1`, selected)
	require.NoError(t, err)
	require.EqualValues(t, 2, queryInt(t, `SELECT revision FROM tickets WHERE id = $1`, selected))
	afterClose := businessState(t)
	replay, err = run(t, fields)
	require.NoError(t, err, "later business state must not erase a committed result")
	require.JSONEq(t, first, replay)

	fields["mapping_evidence_sha256"] = bindEvidenceDigest("altered mapping evidence")
	output, err := run(t, fields)
	require.ErrorContains(t, err, "清单与历史补绑回执冲突")
	require.Empty(t, output)
	require.EqualValues(t, 1, queryInt(t, `SELECT count(*) FROM ticket_legacy_definition_bindings WHERE ticket_id = $1`, selected))
	require.Equal(t, afterClose, businessState(t))

	_, err = testPool.Exec(ctx, down)
	require.ErrorContains(t, err, "retain legacy ticket definition binding receipts")
	require.EqualValues(t, 1, queryInt(t, `SELECT count(*) FROM ticket_legacy_definition_bindings WHERE ticket_id = $1`, selected))
	require.EqualValues(t, 1, queryInt(t, `SELECT definition_version FROM tickets WHERE id = $1`, selected))

	_, err = testPool.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, selected)
	require.NoError(t, err, "the audit receipt must not block normal ticket deletion")
	replay, err = run(t, manifest(selected, "in_progress"))
	require.NoError(t, err, "a historical receipt must survive deletion")
	require.JSONEq(t, first, replay)

	t.Run("concurrent retry returns the same receipt", func(t *testing.T) {
		id := insertTicket(t, "T-106", typeCode, "in_progress")
		input, err := json.Marshal(manifest(id, "in_progress"))
		require.NoError(t, err)
		type outcome struct {
			output string
			err    error
		}
		start := make(chan struct{})
		results := make(chan outcome, 2)
		callContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()
		for range 2 {
			go func() {
				<-start
				var output bytes.Buffer
				err := admincli.RunBindTicketDefinitions(callContext, testPool.Config().ConnString(), bytes.NewReader(input), &output)
				results <- outcome{output: output.String(), err: err}
			}()
		}
		close(start)
		a, b := <-results, <-results
		require.NoError(t, a.err)
		require.NoError(t, b.err)
		require.JSONEq(t, a.output, b.output)
		require.EqualValues(t, 1, queryInt(t, `SELECT count(*) FROM ticket_legacy_definition_bindings WHERE ticket_id = $1`, id))
	})
}

type closedBindOutput struct{}

func (closedBindOutput) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func bindEvidenceDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

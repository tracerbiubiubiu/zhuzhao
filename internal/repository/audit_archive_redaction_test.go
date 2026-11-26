package repository

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/require"
)

// Only the database boundary is replaced; archiveFetchBatch builds the actual JSONL.
type archiveFixtureDB struct {
	archiveDB
	rows pgx.Rows
}

func (db archiveFixtureDB) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return db.rows, nil
}

type archiveFixtureRows struct {
	pgx.Rows
	fields []pgconn.FieldDescription
	values [][]any
	index  int
}

func (r *archiveFixtureRows) Close()                                       {}
func (r *archiveFixtureRows) Err() error                                   { return nil }
func (r *archiveFixtureRows) FieldDescriptions() []pgconn.FieldDescription { return r.fields }
func (r *archiveFixtureRows) Values() ([]any, error)                       { return r.values[r.index-1], nil }
func (r *archiveFixtureRows) Next() bool {
	r.index++
	return r.index <= len(r.values)
}

func TestArchiveFetchBatch_RedactsRiskBodies(t *testing.T) {
	rows := &archiveFixtureRows{
		fields: []pgconn.FieldDescription{
			{Name: "id"}, {Name: "path"}, {Name: "request_body"}, {Name: "response_summary"},
			{Name: "request_id"}, {Name: "username"}, {Name: "status_code"},
		},
		values: [][]any{
			{int64(9007199254740993), "/api/v1/risk-items/dispatch", "synthetic request", "synthetic response", "req-synthetic", "synthetic actor", int32(403)},
			{int64(9007199254740994), "/api/v1/tickets", "ordinary request", "ordinary response", nil, nil, int32(200)},
		},
	}
	batch, err := archiveFetchBatch(context.Background(), archiveFixtureDB{rows: rows}, "audit_logs", time.Now(), 2)
	require.NoError(t, err)
	require.Len(t, batch, 2)
	require.Equal(t, int64(9007199254740993), batch[0].ID)
	require.Equal(t, int64(9007199254740994), batch[1].ID)
	var encodedID struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.Unmarshal(batch[0].Line, &encodedID))
	require.Equal(t, batch[0].ID, encodedID.ID)
	require.JSONEq(t, `{"id":9007199254740993,"path":"/api/v1/risk-items/dispatch","request_body":"","response_summary":"","request_id":"req-synthetic","username":"synthetic actor","status_code":403}`, string(batch[0].Line))
	require.JSONEq(t, `{"id":9007199254740994,"path":"/api/v1/tickets","request_body":"ordinary request","response_summary":"ordinary response","request_id":null,"username":null,"status_code":200}`, string(batch[1].Line))
}

func TestArchiveFetchBatch_RedactionScopeAndMetadata(t *testing.T) {
	for _, tc := range []struct {
		name, table, path string
		body, summary     any
		redacted          bool
	}{
		{"collection", "audit_logs", "/api/v1/risk-items", "request", "response", true},
		{"trailing slash", "audit_logs", "/api/v1/risk-items/", "request", "response", true},
		{"similar ordinary path", "audit_logs", "/api/v1/risky-items", "request", "response", false},
		{"other API version", "audit_logs", "/api/v2/risk-items", "request", "response", false},
		{"null payloads", "audit_logs", "/api/v1/risk-items/1", nil, nil, true},
		{"empty payloads", "audit_logs", "/api/v1/risk-items/1", "", "", true},
		{"ordinary", "audit_logs", "/api/v1/tickets", "request", "response", false},
		{"empty path", "audit_logs", "", nil, "response", false},
		{"policy table", "policy_evaluation_logs", "/api/v1/risk-items/1", "request", "response", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			created := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
			rows := &archiveFixtureRows{
				fields: []pgconn.FieldDescription{
					{Name: "id"}, {Name: "path"}, {Name: "request_body"}, {Name: "response_summary"},
					{Name: "user_id"}, {Name: "username"}, {Name: "employee_no"}, {Name: "real_name"},
					{Name: "method"}, {Name: "status_code"}, {Name: "duration"}, {Name: "ip"},
					{Name: "user_agent"}, {Name: "request_id"}, {Name: "created_at"},
					{Name: "future_metadata", DataTypeOID: 3802}, {Name: "json_metadata", DataTypeOID: 114},
				},
				values: [][]any{{int64(41), tc.path, tc.body, tc.summary, int64(7), "synthetic", "S000007", "Synthetic Actor",
					"POST", int32(403), int64(123), "192.0.2.1", "synthetic-agent", "req-synthetic", created,
					[]byte(`{"nested":[1,true,null]}`), []byte(`{"value":"unchanged"}`)}},
			}
			batch, err := archiveFetchBatch(context.Background(), archiveFixtureDB{rows: rows}, tc.table, created.Add(time.Hour), 1)
			require.NoError(t, err)
			require.Len(t, batch, 1)
			require.Equal(t, int64(41), batch[0].ID)
			var got map[string]any
			require.NoError(t, json.Unmarshal(batch[0].Line, &got))
			body, summary := tc.body, tc.summary
			if tc.redacted {
				body, summary = "", ""
			}
			require.Equal(t, map[string]any{
				"id": float64(41), "path": tc.path, "request_body": body, "response_summary": summary,
				"user_id": float64(7), "username": "synthetic", "employee_no": "S000007", "real_name": "Synthetic Actor",
				"method": "POST", "status_code": float64(403), "duration": float64(123), "ip": "192.0.2.1",
				"user_agent": "synthetic-agent", "request_id": "req-synthetic", "created_at": "2026-01-02T03:04:05Z",
				"future_metadata": map[string]any{"nested": []any{float64(1), true, nil}},
				"json_metadata":   map[string]any{"value": "unchanged"},
			}, got)
			// Serialization must not mutate the fetched payloads or deletion identity.
			require.Equal(t, tc.body, rows.values[0][2])
			require.Equal(t, tc.summary, rows.values[0][3])
		})
	}
}

func TestArchiveBatch_RejectsUnallowlistedTables(t *testing.T) {
	// A nil database would panic on access: rejection must precede any SQL.
	for _, table := range []string{"", "users", "audit_logs; DELETE FROM audit_logs", "public.audit_logs"} {
		t.Run(table, func(t *testing.T) {
			_, err := archiveFetchBatch(context.Background(), nil, table, time.Now(), 1)
			require.ErrorContains(t, err, "not allowlisted")
			require.ErrorContains(t, archiveDeleteBatch(context.Background(), nil, table, []int64{41}), "not allowlisted")
		})
	}
}

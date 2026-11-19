//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

func c02Data(t *testing.T, data map[string]any) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(data)
	require.NoError(t, err)
	return raw
}

// Wrong-purpose binding must roll back the complete Create, just as Update does.
func TestC02_CreateFieldPurposes(t *testing.T) {
	for _, tc := range []struct {
		field, purpose string
		valid          bool
	}{
		{"docs", "attachment_create", true}, {"docs", "richtext_create", false}, {"docs", "avatar", false},
		{"body", "richtext_create", true}, {"body", "attachment_create", false},
	} {
		t.Run(tc.field+"_"+tc.purpose, func(t *testing.T) {
			env := newB53Env(t)
			ctx := context.Background()
			code := deliveryFileType(t, env)
			key := env.confirmedCreate(t, env.u1, tc.purpose)
			req := env.createReq(nil)
			req.TypeCode = code
			value := any([]string{key})
			if tc.field == "body" {
				value = "![](/api/v1/files/" + key + ")"
			}
			req.CustomData = c02Data(t, map[string]any{tc.field: value})
			tk, err := env.svc.Create(ctx, req, env.u1)
			if tc.valid {
				require.NoError(t, err)
				require.NotNil(t, tk)
				return
			}
			require.Error(t, err)
			require.Equal(t, errcode.ErrInvalidParams.Code, b53Code(t, err))
			var n int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE type_code=$1`, code).Scan(&n))
			require.Zero(t, n)
			var purpose string
			require.NoError(t, testPool.QueryRow(ctx, `SELECT purpose FROM file_objects WHERE bucket='b53' AND object_key=$1`, key).Scan(&purpose))
			require.Equal(t, tc.purpose, purpose)
		})
	}
}

// The same object arriving from explicit and field sources counts only once.
func TestC02_CreateDeduplicatesSources(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := deliveryFileType(t, env)
	_, err := env.svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{
		Code: code,
		Fields: []model.TicketTypeFieldInput{
			{FieldKey: "docs", FieldLabel: "docs", FieldType: "file"},
			{FieldKey: "body", FieldLabel: "body", FieldType: "richtext"},
			{FieldKey: "more_docs", FieldLabel: "more docs", FieldType: "file"},
		},
	})
	require.NoError(t, err)
	env.stgCfg.MaxAttachmentsPerTicket = 1
	key := env.confirmedCreate(t, env.u1, "attachment_create")
	req := env.createReq([]string{key})
	req.TypeCode = code
	req.CustomData = c02Data(t, map[string]any{"docs": []string{key}, "more_docs": []string{key}})
	tk, err := env.svc.Create(ctx, req, env.u1)
	require.NoError(t, err)
	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
	require.Equal(t, 1, n)
	for _, field := range []string{"docs", "more_docs"} {
		require.Equal(t, errcode.ErrConflict.Code, b53Code(t, env.stg.DeleteTicketAttachment(ctx, tk.ID, key)))
		_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: c02Data(t, map[string]any{field: nil})}, env.u1)
		require.NoError(t, err)
	}
	require.NoError(t, env.stg.DeleteTicketAttachment(ctx, tk.ID, key))
}

func TestC02_AttachmentValidationDoesNotWaitForAvatarObjectLock(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := deliveryFileType(t, env)
	avatar := env.confirmedCreate(t, env.u1, "avatar")
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	var id int64
	require.NoError(t, tx.QueryRow(ctx,
		`SELECT id FROM file_objects WHERE bucket='b53' AND object_key=$1 FOR UPDATE`, avatar).Scan(&id))
	req := env.createReq(nil)
	req.TypeCode = code
	req.CustomData = c02Data(t, map[string]any{"docs": []string{avatar}})

	requestCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	_, err = env.svc.Create(requestCtx, req, env.u1)
	require.Equal(t, errcode.ErrInvalidParams.Code, b53Code(t, err))
	require.NoError(t, requestCtx.Err(), "validation should exclude unrelated objects before taking locks")
}

// Appending B retains A; removing a field reference retains both attachments.
func TestC02_DeleteReferencedAttachment(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := deliveryFileType(t, env)
	tk := deliveryFileTicket(t, env, code)
	a := env.confirmedCreate(t, env.u1, "attachment_create")
	b := env.confirmedCreate(t, env.u1, "attachment_create")
	for _, keys := range [][]string{{a}, {a, b}} {
		_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: c02Data(t, map[string]any{"docs": keys})}, env.u1)
		require.NoError(t, err)
	}
	err := env.stg.DeleteTicketAttachment(ctx, tk.ID, a)
	require.Equal(t, errcode.ErrConflict.Code, b53Code(t, err))
	require.NotContains(t, err.Error(), "docs")
	var n int
	var status string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
	require.Equal(t, 2, n)
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket='b53' AND object_key=$1`, a).Scan(&status))
	require.Equal(t, "confirmed", status)
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: json.RawMessage(`{"docs":null}`)}, env.u1)
	require.NoError(t, err)
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
	require.Equal(t, 2, n)
	require.NoError(t, env.stg.DeleteTicketAttachment(ctx, tk.ID, a))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket='b53' AND object_key=$1`, a).Scan(&status))
	require.Equal(t, "deleted", status)
}

type c02RemoveClient struct {
	b53StubClient
	calls  int
	remove func(context.Context, string) error
}

func (c *c02RemoveClient) RemoveObject(ctx context.Context, key string) error {
	c.calls++
	if c.remove != nil {
		return c.remove(ctx, key)
	}
	return nil
}

// A DB failure after unlink must restore the relation and never remove bytes.
func TestC02_DeleteRollbackAndPostCommitCleanup(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	tk := deliveryFileTicket(t, env, deliveryFileType(t, env))
	key := env.confirmedCreate(t, env.u1, "attachment_create")
	_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Attachments: []string{key}}, env.u1)
	require.NoError(t, err)
	client := &c02RemoveClient{}
	svc := storageservice.NewService(repository.NewStorageRepo(testPool), client, pkgstorage.NewConfigStore(env.stgCfg), slog.Default())
	name := "c02_fail_" + uniqueSuffix()
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF OLD.object_key='%s' AND NEW.status='deleted' THEN RAISE EXCEPTION 'synthetic mark failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE UPDATE ON file_objects FOR EACH ROW EXECUTE FUNCTION %s()`, name, key, name, name))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON file_objects; DROP FUNCTION IF EXISTS %s()`, name, name))
	})
	err = svc.DeleteTicketAttachment(ctx, tk.ID, key)
	require.Error(t, err)
	require.Zero(t, client.calls)
	var n int
	var status string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
	require.Equal(t, 1, n)
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket='b53' AND object_key=$1`, key).Scan(&status))
	require.Equal(t, "confirmed", status)
	_, err = testPool.Exec(ctx, fmt.Sprintf(`DROP TRIGGER %s ON file_objects; DROP FUNCTION %s()`, name, name))
	require.NoError(t, err)
	client.remove = func(ctx context.Context, key string) error {
		require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket='b53' AND object_key=$1`, key).Scan(&status))
		require.Equal(t, "deleted", status)
		require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
		require.Zero(t, n)
		return errors.New("synthetic storage failure")
	}
	require.NoError(t, svc.DeleteTicketAttachment(ctx, tk.ID, key))
	require.Equal(t, 1, client.calls)
}

func TestC02_DeleteAssociationBeforePurpose(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := deliveryFileType(t, env)
	first := deliveryFileTicket(t, env, code)
	second := deliveryFileTicket(t, env, code)
	key := env.confirmedCreate(t, env.u1, "richtext_create")
	_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: first.ID, Attachments: []string{key}}, env.u1)
	require.NoError(t, err)
	require.Equal(t, errcode.ErrFileNotFound.Code, b53Code(t, env.stg.DeleteTicketAttachment(ctx, second.ID, key)))
	require.Equal(t, errcode.ErrInvalidParams.Code, b53Code(t, env.stg.DeleteTicketAttachment(ctx, first.ID, key)))
	require.Equal(t, errcode.ErrFileNotFound.Code, b53Code(t, env.stg.DeleteTicketAttachment(ctx, first.ID, "missing")))
}

// Synthetic legacy states exercise the destructive boundary without generating
// invalid data through product writes: all real fields count, and ambiguity holds.
func TestC02_DeleteChecksStoredReferencesConservatively(t *testing.T) {
	for _, tc := range []string{"richtext", "malformed_file", "unknown_definition", "null_key", "empty_key"} {
		t.Run(tc, func(t *testing.T) {
			env := newB53Env(t)
			ctx := context.Background()
			tk := deliveryFileTicket(t, env, deliveryFileType(t, env))
			key := env.confirmedCreate(t, env.u1, "attachment_create")
			_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Attachments: []string{key}}, env.u1)
			require.NoError(t, err)
			data := map[string]any{"body": "![](/api/v1/files/" + key + ")"}
			if tc == "null_key" {
				data = map[string]any{"docs": []any{nil}}
			}
			if tc == "empty_key" {
				data = map[string]any{"docs": []string{""}}
			}
			if tc == "malformed_file" {
				data = map[string]any{"docs": 42}
			}
			if tc == "unknown_definition" {
				data = map[string]any{"retired_docs": []string{key}}
			}
			_, err = testPool.Exec(ctx, `UPDATE tickets SET custom_data=$2 WHERE id=$1`, tk.ID, []byte(c02Data(t, data)))
			require.NoError(t, err)
			require.Error(t, env.stg.DeleteTicketAttachment(ctx, tk.ID, key))
			var n int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
			require.Equal(t, 1, n)
		})
	}
}

// Hold the ticket row, publish a newly referenced same-ticket object, then let
// deletion continue. It must observe the committed reference, for eight rounds.
func TestC02_DeleteReadsReferencesAfterTicketLock(t *testing.T) {
	env := newB53Env(t)
	code := deliveryFileType(t, env)
	for round := 0; round < 8; round++ {
		t.Run(fmt.Sprint(round), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			tk := deliveryFileTicket(t, env, code)
			key := env.confirmedCreate(t, env.u1, "attachment_create")
			_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Attachments: []string{key}}, env.u1)
			require.NoError(t, err)
			tx, err := testPool.Begin(ctx)
			require.NoError(t, err)
			defer tx.Rollback(context.Background())
			var id int64
			require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, tk.ID).Scan(&id))
			var pid int
			require.NoError(t, tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid))
			done := make(chan error, 1)
			go func() { done <- env.stg.DeleteTicketAttachment(ctx, tk.ID, key) }()
			require.Eventually(t, func() bool {
				var waiting bool
				err := testPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE $1=ANY(pg_blocking_pids(pid)))`, pid).Scan(&waiting)
				return err == nil && waiting
			}, 2*time.Second, 5*time.Millisecond, "delete must wait for ticket row before checking references")
			// Fixture transaction models the locked commit of a field writer.
			_, err = tx.Exec(ctx, `UPDATE tickets SET custom_data=$2 WHERE id=$1`, tk.ID, []byte(c02Data(t, map[string]any{"docs": []string{key}})))
			require.NoError(t, err)
			require.NoError(t, tx.Commit(ctx))
			select {
			case err = <-done:
				require.Equal(t, errcode.ErrConflict.Code, b53Code(t, err))
			case <-ctx.Done():
				t.Fatal(ctx.Err())
			}
		})
	}
}

func TestC02_DeleteEmptyCustomData(t *testing.T) {
	for _, state := range []string{"sql_null", "json_null"} {
		t.Run(state, func(t *testing.T) {
			env := newB53Env(t)
			ctx := context.Background()
			key := env.confirmedCreate(t, env.u1, "attachment_create")
			req := env.createReq([]string{key})
			if state == "json_null" {
				req.CustomData = json.RawMessage(`null`)
			}
			tk, err := env.svc.Create(ctx, req, env.u1)
			require.NoError(t, err)
			require.NoError(t, env.stg.DeleteTicketAttachment(ctx, tk.ID, key), "legal empty fields must not block independent attachments")
		})
	}
}

func TestC02_DeleteHoldsAmbiguousHistoricalBinding(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := deliveryFileType(t, env)
	first := deliveryFileTicket(t, env, code)
	second := deliveryFileTicket(t, env, code)
	key := env.confirmedCreate(t, env.u1, "attachment_create")
	_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: first.ID, Attachments: []string{key}}, env.u1)
	require.NoError(t, err)
	// Synthetic pre-repair dual binding; this test does not ask product writes to create it.
	_, err = testPool.Exec(ctx, `INSERT INTO ticket_attachments(ticket_id,file_object_id,uploaded_by) SELECT $1,id,$2 FROM file_objects WHERE bucket='b53' AND object_key=$3`, second.ID, env.u1, key)
	require.NoError(t, err)
	require.Equal(t, errcode.ErrConflict.Code, b53Code(t, env.stg.DeleteTicketAttachment(ctx, first.ID, key)))
	var n int
	var status string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=ANY($1)`, []int64{first.ID, second.ID}).Scan(&n))
	require.Equal(t, 2, n)
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket='b53' AND object_key=$1`, key).Scan(&status))
	require.Equal(t, "confirmed", status)
}

// Explicit input must not normalize contradictory metadata or preexisting
// relationships before validating the same object's field-purpose reference.
func TestC02_CombinedSourcesPreserveDanglingIntegrity(t *testing.T) {
	for _, operation := range []string{"create", "update"} {
		for _, state := range []string{"contradictory_owner", "prior_binding"} {
			t.Run(operation+"_"+state, func(t *testing.T) {
				env := newB53Env(t)
				ctx := context.Background()
				code := deliveryFileType(t, env)
				prior := deliveryFileTicket(t, env, code)
				target := deliveryFileTicket(t, env, code)
				key := env.confirmedCreate(t, env.u1, "attachment_create")
				if state == "contradictory_owner" {
					_, err := testPool.Exec(ctx, `UPDATE file_objects SET owner_kind='user',owner_id='999999' WHERE bucket='b53' AND object_key=$1`, key)
					require.NoError(t, err)
				} else {
					_, err := testPool.Exec(ctx, `INSERT INTO ticket_attachments(ticket_id,file_object_id,uploaded_by) SELECT $1,id,$2 FROM file_objects WHERE bucket='b53' AND object_key=$3`, prior.ID, env.u1, key)
					require.NoError(t, err)
				}
				data := c02Data(t, map[string]any{"docs": []string{key}})
				var err error
				if operation == "create" {
					req := env.createReq([]string{key})
					req.TypeCode = code
					req.CustomData = data
					_, err = env.svc.Create(ctx, req, env.u1)
				} else {
					_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: target.ID, Attachments: []string{key}, CustomData: data}, env.u1)
				}
				require.Equal(t, errcode.ErrInvalidParams.Code, b53Code(t, err))
				var purpose string
				require.NoError(t, testPool.QueryRow(ctx, `SELECT purpose FROM file_objects WHERE bucket='b53' AND object_key=$1`, key).Scan(&purpose))
				require.Equal(t, "attachment_create", purpose)
				var n int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, target.ID).Scan(&n))
				require.Zero(t, n)
			})
		}
	}
}

func TestC02_DeleteOwnerMetadataCompatibility(t *testing.T) {
	for _, legacy := range []bool{true, false} {
		t.Run(fmt.Sprint(legacy), func(t *testing.T) {
			env := newB53Env(t)
			ctx := context.Background()
			tk := deliveryFileTicket(t, env, deliveryFileType(t, env))
			key := env.confirmedCreate(t, env.u1, "attachment_create")
			_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Attachments: []string{key}}, env.u1)
			require.NoError(t, err)
			if legacy {
				_, err = testPool.Exec(ctx, `UPDATE file_objects SET owner_kind=NULL,owner_id=NULL WHERE bucket='b53' AND object_key=$1`, key)
			} else {
				_, err = testPool.Exec(ctx, `UPDATE file_objects SET owner_kind='user',owner_id='999999' WHERE bucket='b53' AND object_key=$1`, key)
			}
			require.NoError(t, err)
			err = env.stg.DeleteTicketAttachment(ctx, tk.ID, key)
			if legacy {
				require.NoError(t, err)
			} else {
				require.Equal(t, errcode.ErrConflict.Code, b53Code(t, err))
			}
		})
	}
}

//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// Deleted objects stay invalid. Recovery is an explicit custom_data patch, not
// a relaxation of confirmation, ownership or the schema's required constraint.
func TestFieldReferenceRecovery_AfterAttachmentDeletion(t *testing.T) {
	for _, required := range []bool{false, true} {
		name := "optional"
		if required {
			name = "required"
		}
		t.Run(name, func(t *testing.T) {
			env := newB53Env(t)
			ctx := context.Background()
			code := deliveryFileType(t, env)
			if required {
				_, err := env.svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{
					{FieldKey: "docs", FieldLabel: "docs", FieldType: "file", Required: true},
					{FieldKey: "body", FieldLabel: "body", FieldType: "richtext"},
				}})
				require.NoError(t, err)
				publishD01(t, env.svc, code, env.u1)
			}
			key := env.confirmedCreate(t, env.u1, "attachment_create")
			raw := c02Data(t, map[string]any{"docs": []string{key}, "body": "unchanged"})
			req := env.createReq(nil)
			req.TypeCode = code
			req.CustomData = raw
			tk, err := env.svc.Create(ctx, req, env.u1)
			require.NoError(t, err)
			// Synthetic pre-repair history: product DELETE must never create this
			// broken reference. Load the old state directly in this isolated DB.
			_, err = testPool.Exec(ctx, `DELETE FROM ticket_attachments WHERE ticket_id=$1`, tk.ID)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `UPDATE file_objects SET status='deleted' WHERE bucket='b53' AND object_key=$1`, key)
			require.NoError(t, err)
			var status string
			var count int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket='b53' AND object_key=$1`, key).Scan(&status))
			require.Equal(t, "deleted", status)
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&count))
			require.Zero(t, count)

			title := "title-only edit after unlink"
			_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: &title}, env.u1)
			require.NoError(t, err, "title-only editing does not resubmit historical field references")
			before, err := env.svc.ticketRepo.GetByID(ctx, tk.ID)
			require.NoError(t, err)
			badTitle := "must roll back with stale field reference"
			_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: &badTitle, CustomData: json.RawMessage(`{"body":"other field"}`)}, env.u1)
			require.Error(t, err, "merged custom_data must not retain an object already deleted")
			after, err := env.svc.ticketRepo.GetByID(ctx, tk.ID)
			require.NoError(t, err)
			require.Equal(t, before.Title, after.Title)
			require.JSONEq(t, string(before.CustomData), string(after.CustomData))

			_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: json.RawMessage(`{"docs":null,"body":"repaired"}`)}, env.u1)
			if required {
				require.Error(t, err, "recovery must not bypass a required file field")
				saved, getErr := env.svc.ticketRepo.GetByID(ctx, tk.ID)
				require.NoError(t, getErr)
				require.Equal(t, before.Title, saved.Title)
				require.JSONEq(t, string(before.CustomData), string(saved.CustomData))
			} else {
				require.NoError(t, err)
				saved, getErr := env.svc.ticketRepo.GetByID(ctx, tk.ID)
				require.NoError(t, getErr)
				var data map[string]any
				require.NoError(t, json.Unmarshal(saved.CustomData, &data))
				require.NotContains(t, data, "docs")
				require.Equal(t, "repaired", data["body"])
			}
			fresh := env.confirmedCreate(t, env.u1, "attachment_create")
			replacement, err := json.Marshal(map[string]any{"docs": []string{fresh}, "body": "replacement"})
			require.NoError(t, err)
			_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: replacement}, env.u1)
			require.NoError(t, err)
			saved, err := env.svc.ticketRepo.GetByID(ctx, tk.ID)
			require.NoError(t, err)
			var data map[string]any
			require.NoError(t, json.Unmarshal(saved.CustomData, &data))
			require.Equal(t, []any{fresh}, data["docs"])
			_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: raw}, env.u1)
			require.Error(t, err, "the deleted original can never be reintroduced by a repair")
			afterRejected, err := env.svc.ticketRepo.GetByID(ctx, tk.ID)
			require.NoError(t, err)
			require.Equal(t, saved.Title, afterRejected.Title)
			require.JSONEq(t, string(saved.CustomData), string(afterRejected.CustomData))
		})
	}
}

//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestFieldReferenceRepair_LegacyNullableOwnerMetadata(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := deliveryFileType(t, env)
	tk := deliveryFileTicket(t, env, code)
	key := env.confirmedCreate(t, env.u1, "attachment_create")
	raw, err := json.Marshal(map[string]any{"docs": []string{key}})
	require.NoError(t, err)
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: raw}, env.u1)
	require.NoError(t, err)
	// Migration 000047 intentionally did not backfill old owner metadata; the
	// authoritative same-ticket binding remains valid for a legal second editor.
	_, err = testPool.Exec(ctx, `UPDATE file_objects SET owner_kind=NULL,owner_id=NULL WHERE bucket='b53' AND object_key=$1`, key)
	require.NoError(t, err)
	env.svc.roleFetcher.(stubRoleFetcher)[env.u2] = []string{"admin"}
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: raw}, env.u2)
	require.NoError(t, err)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&count))
	require.Equal(t, 1, count)
	// A contradictory nonempty owner is not the historical NULL compatibility.
	_, err = testPool.Exec(ctx, `UPDATE file_objects SET owner_kind='user',owner_id='999999' WHERE bucket='b53' AND object_key=$1`, key)
	require.NoError(t, err)
	title := "must not change"
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: &title, CustomData: raw}, env.u2)
	require.Error(t, err)
	saved, err := env.svc.ticketRepo.GetByID(ctx, tk.ID)
	require.NoError(t, err)
	require.Equal(t, tk.Title, saved.Title)
}

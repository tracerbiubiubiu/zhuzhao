//go:build integration

package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// TestGC_TicketBindingWins waits for a real binding transaction while GC still sees
// its old unbound snapshot. After the object lock, GC must notice the committed reference.
func TestGC_TicketBindingWins(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ticketID := insertTicket(t, f.uid)
	key := "tickets/" + uniqueSuffix() + ".png"
	_, err := testPool.Exec(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by,created_at,confirmed_at) VALUES($1,$2,'attachment_create','confirmed','a.png','image/png',$3,NOW()-interval '30 days',NOW()-interval '30 days')`, f.bucket, key, f.uid)
	require.NoError(t, err)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticketID)
	require.NoError(t, err)
	require.NoError(t, f.repo.BindCreateAttachments(ctx, tx, f.bucket, f.uid, ticketID, []string{key}, 20))
	done := make(chan error, 1)
	go func() { done <- storageservice.NewGCJob(f.repo, f.client, f.cfg, testLogger(), true).Handle(ctx, nil) }()
	waitAvatarLock(t, ctx, "%file_objects%", 1)
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, <-done)
	var status string
	var refs int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, f.bucket, key).Scan(&status))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments ta JOIN file_objects fo ON fo.id=ta.file_object_id WHERE fo.bucket=$1 AND fo.object_key=$2`, f.bucket, key).Scan(&refs))
	require.Equal(t, "confirmed", status)
	require.Equal(t, 1, refs)
	require.False(t, f.client.wasRemoved(key))
}

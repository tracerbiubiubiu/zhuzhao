//go:build integration

package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// Retiring someone else's historical key is the break this test detects.
func TestC02AvatarForeignOld(t *testing.T) {
	for _, owner := range []string{"foreign_creator", "foreign_owner"} {
		t.Run(owner, func(t *testing.T) {
			f := newAvatarRaceFixture(t)
			ctx := context.Background()
			old, next := f.avatar(t), f.avatar(t)
			var other int64
			eno := "E8C02" + uniqueSuffix()
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(employee_no,username,password) VALUES($1,$1,'hash') RETURNING id`, eno).Scan(&other))
			t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM users WHERE id=$1`, other) })
			if owner == "foreign_creator" {
				_, err := testPool.Exec(ctx, `UPDATE file_objects SET created_by=$3 WHERE bucket=$1 AND object_key=$2`, f.bucket, old, other)
				require.NoError(t, err)
			} else {
				_, err := testPool.Exec(ctx, `UPDATE file_objects SET owner_kind='user',owner_id=$3::bigint::text WHERE bucket=$1 AND object_key=$2`, f.bucket, old, other)
				require.NoError(t, err)
			}
			_, err := testPool.Exec(ctx, `UPDATE users SET avatar=$2 WHERE id=$1`, f.uid, old)
			require.NoError(t, err)
			_, err = f.svc.BindAvatar(ctx, next, f.uid)
			require.NoError(t, err)
			var status string
			require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, f.bucket, old).Scan(&status))
			require.Equal(t, "confirmed", status, "foreign historical avatar must remain confirmed")
			require.False(t, f.client.wasRemoved(old))
		})
	}
}

func TestC02AvatarRejectsContradictoryNewOwner(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx := context.Background()
	key := f.avatar(t)
	_, err := testPool.Exec(ctx, `UPDATE file_objects SET owner_kind='ticket',owner_id='123' WHERE bucket=$1 AND object_key=$2`, f.bucket, key)
	require.NoError(t, err)
	_, err = f.svc.BindAvatar(ctx, key, f.uid)
	require.Error(t, err, "binding must not overwrite contradictory ownership")
	var avatar string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT coalesce(avatar,'') FROM users WHERE id=$1`, f.uid).Scan(&avatar))
	require.Empty(t, avatar)
}

// A second user's historical reference commits while retirement waits on the
// old object. The retirement statement's original snapshot must not win.
func TestC02AvatarOldReferenceAfterLock(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	old, next := f.avatar(t), f.avatar(t)
	_, err := f.svc.BindAvatar(ctx, old, f.uid)
	require.NoError(t, err)
	var other int64
	eno := "E8C02" + uniqueSuffix()
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(employee_no,username,password) VALUES($1,$1,'hash') RETURNING id`, eno).Scan(&other))
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, other) })
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2 FOR UPDATE`, f.bucket, old)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { _, e := f.svc.BindAvatar(ctx, next, f.uid); done <- e }()
	waitAvatarLock(t, ctx, "%file_objects%", 1)
	_, err = tx.Exec(ctx, `UPDATE users SET avatar=$2 WHERE id=$1`, other, old)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, <-done)
	var status string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, f.bucket, old).Scan(&status))
	require.Equal(t, "confirmed", status, "post-lock reference must protect old avatar")
	require.False(t, f.client.wasRemoved(old))
}

type c02InterruptedRemove struct {
	avatarRaceClient
	cancel context.CancelFunc
	status string
	id     int64
}

func (c *c02InterruptedRemove) RemoveObject(ctx context.Context, key string) error {
	_ = testPool.QueryRow(ctx, `SELECT id,status FROM file_objects WHERE object_key=$1`, key).Scan(&c.id, &c.status)
	c.cancel()
	return errors.New("synthetic interruption")
}
func TestC02PendingDurableBeforeRemove(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	key := f.avatar(t)
	var id int64
	require.NoError(t, testPool.QueryRow(ctx, `UPDATE file_objects SET status='pending',confirmed_at=NULL WHERE bucket=$1 AND object_key=$2 RETURNING id`, f.bucket, key).Scan(&id))
	client := &c02InterruptedRemove{cancel: cancel}
	_ = storageservice.NewGCJob(f.repo, client, f.cfg, testLogger(), true).Handle(ctx, nil)
	require.Equal(t, "deleted", client.status, "deleted tombstone must commit before physical removal")
	require.Equal(t, id, client.id, "pending row identity must survive retirement")
	var status string
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT status FROM file_objects WHERE id=$1`, id).Scan(&status))
	require.Equal(t, "deleted", status)
	require.Error(t, f.repo.Confirm(context.Background(), id, 10, "e"))
	require.NoError(t, storageservice.NewGCJob(f.repo, f.client, f.cfg, testLogger(), true).Handle(context.Background(), nil))
	require.True(t, f.client.wasRemoved(key))
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT count(*) FROM file_objects WHERE id=$1`, id).Scan(&n))
	require.Zero(t, n)
}

func TestC02PendingConcurrentConfirmWins(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	key := f.avatar(t)
	var id int64
	require.NoError(t, testPool.QueryRow(ctx, `UPDATE file_objects SET status='pending',confirmed_at=NULL WHERE bucket=$1 AND object_key=$2 RETURNING id`, f.bucket, key).Scan(&id))
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `UPDATE file_objects SET status='confirmed',confirmed_at=NOW() WHERE id=$1`, id)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- storageservice.NewGCJob(f.repo, f.client, f.cfg, testLogger(), true).Handle(ctx, nil) }()
	waitAvatarLock(t, ctx, "%file_objects%", 1)
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, <-done)
	var status string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE id=$1`, id).Scan(&status))
	require.Equal(t, "confirmed", status, "concurrent Confirm must retain its row and bytes")
	require.False(t, f.client.wasRemoved(key))
}

func TestC02HistoricalTicketReferences(t *testing.T) {
	for _, shape := range []string{"file", "richtext", "unknown", "orphan"} {
		t.Run(shape, func(t *testing.T) {
			f := newAvatarRaceFixture(t)
			ctx := context.Background()
			tid := insertTicket(t, f.uid)
			key := "tickets/" + uniqueSuffix() + ".png"
			_, err := testPool.Exec(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by,confirmed_at) VALUES($1,$2,'ticket_richtext','confirmed','x','image/png',$3,NOW()-interval '30 days')`, f.bucket, key, f.uid)
			require.NoError(t, err)
			var raw string
			switch shape {
			case "file":
				raw = `{"upload":["` + key + `"]}`
			case "richtext":
				raw = `{"body":"![x](/api/v1/files/` + key + `)"}`
			case "unknown":
				raw = `{"hidden":{"old":"` + key + `"}}`
			default:
				raw = `{"other":"unrelated"}`
			}
			_, err = testPool.Exec(ctx, `UPDATE tickets SET custom_data=$2 WHERE id=$1`, tid, raw)
			require.NoError(t, err)
			require.NoError(t, storageservice.NewGCJob(f.repo, f.client, f.cfg, testLogger(), true).Handle(ctx, nil))
			var status string
			require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, f.bucket, key).Scan(&status))
			if shape == "orphan" {
				require.Equal(t, "deleted", status)
				require.True(t, f.client.wasRemoved(key))
			} else {
				require.Equal(t, "confirmed", status, "historical field reference without attachment relation must protect bytes")
				require.False(t, f.client.wasRemoved(key))
			}
		})
	}
}

func TestC02HistoricalReferenceAfterObjectLock(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	tid := insertTicket(t, f.uid)
	key := "tickets/" + uniqueSuffix() + ".png"
	_, err := testPool.Exec(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by,confirmed_at) VALUES($1,$2,'ticket_attachment','confirmed','x','image/png',$3,NOW()-interval '30 days')`, f.bucket, key, f.uid)
	require.NoError(t, err)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(context.Background())
	_, err = tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, tid)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2 FOR UPDATE`, f.bucket, key)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- storageservice.NewGCJob(f.repo, f.client, f.cfg, testLogger(), true).Handle(ctx, nil) }()
	waitAvatarLock(t, ctx, "%file_objects%", 1)
	_, err = tx.Exec(ctx, `UPDATE tickets SET custom_data=jsonb_build_object('file',jsonb_build_array($2::text)) WHERE id=$1`, tid, key)
	require.NoError(t, err)
	require.NoError(t, tx.Commit(ctx))
	require.NoError(t, <-done)
	var status string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, f.bucket, key).Scan(&status))
	require.Equal(t, "confirmed", status, "reference committed during object wait must protect bytes")
	require.False(t, f.client.wasRemoved(key))
}

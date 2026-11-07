//go:build integration

package storage_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// avatarRaceClient isolates concurrent cleanup observations from the shared sequential stub.
type avatarRaceClient struct {
	stubClient
	mu   sync.Mutex
	keys []string
}

func (c *avatarRaceClient) RemoveObject(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.keys = append(c.keys, key)
	return nil
}
func (c *avatarRaceClient) wasRemoved(key string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, k := range c.keys {
		if k == key {
			return true
		}
	}
	return false
}

type avatarRaceFixture struct {
	uid    int64
	bucket string
	svc    *storageservice.Service
	repo   *repository.StorageRepo
	client *avatarRaceClient
	cfg    *pkgstorage.ConfigStore
}

func newAvatarRaceFixture(t *testing.T) *avatarRaceFixture {
	t.Helper()
	f := &avatarRaceFixture{bucket: "avatar-race-" + uniqueSuffix()}
	ctx := context.Background()
	eno := "E8" + uniqueSuffix()
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(employee_no,username,password,status) VALUES($1,$1,'synthetic-hash',1) RETURNING id`, eno).Scan(&f.uid))
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM users WHERE id=$1`, f.uid) })
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, f.bucket) })
	cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "http://stub", Bucket: f.bucket, AccessKey: "synthetic-ak", SecretKey: "synthetic-sk", MaxAttachmentsPerTicket: 1}
	cfg.ApplyDefaults()
	f.cfg = pkgstorage.NewConfigStore(cfg)
	f.repo = repository.NewStorageRepo(testPool)
	f.client = &avatarRaceClient{stubClient: stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}}
	f.svc = storageservice.NewService(f.repo, f.client, f.cfg, testLogger())
	return f
}
func (f *avatarRaceFixture) avatar(t *testing.T) string {
	t.Helper()
	key := "avatars/" + uniqueSuffix() + ".png"
	_, err := testPool.Exec(context.Background(), `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by,created_at,confirmed_at) VALUES($1,$2,'avatar','confirmed','a.png','image/png',$3,NOW()-interval '30 days',NOW()-interval '30 days')`, f.bucket, key, f.uid)
	require.NoError(t, err)
	return key
}
func (f *avatarRaceFixture) assertCurrent(t *testing.T, key string) {
	t.Helper()
	var avatar, status string
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT u.avatar,fo.status FROM users u JOIN file_objects fo ON fo.object_key=u.avatar AND fo.bucket=$2 WHERE u.id=$1`, f.uid, f.bucket).Scan(&avatar, &status))
	require.Equal(t, key, avatar)
	require.Equal(t, "confirmed", status)
	require.False(t, f.client.wasRemoved(key), "current avatar must survive physical cleanup")
}
func waitAvatarLock(t *testing.T, ctx context.Context, pattern string, n int) {
	t.Helper()
	require.Eventually(t, func() bool {
		var count int
		err := testPool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE $1`, pattern).Scan(&count)
		return err == nil && count >= n
	}, 3*time.Second, 5*time.Millisecond, fmt.Sprintf("waiting for %d locks matching %s", n, pattern))
}

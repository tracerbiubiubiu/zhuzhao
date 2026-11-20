//go:build integration

package storage_test

import (
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Proxy-valued historical references must protect bytes in both retirement and
// the active avatar orphan sweep; exact raw-key-only checks lose these objects.
func TestC02ReviewHistoricalAvatarProxyLives(t *testing.T) {
	for _, prefix := range []string{"/api/v1/files/", "https://app.example.invalid/api/v1/files/"} {
		t.Run(prefix, func(t *testing.T) {
			f := newAvatarRaceFixture(t)
			ctx := context.Background()
			old, next := f.avatar(t), f.avatar(t)
			_, err := f.svc.BindAvatar(ctx, old, f.uid)
			require.NoError(t, err)
			var other int64
			eno := "E8CR" + uniqueSuffix()
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(employee_no,username,password,avatar) VALUES($1,$1,'hash',$2) RETURNING id`, eno, prefix+old).Scan(&other))
			t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM users WHERE id=$1`, other) })
			_, err = f.svc.BindAvatar(ctx, next, f.uid)
			require.NoError(t, err)
			require.NoError(t, storageservice.NewGCJob(f.repo, f.client, f.cfg, testLogger(), false).Handle(ctx, nil))
			var status string
			require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, f.bucket, old).Scan(&status))
			require.Equal(t, "confirmed", status)
			require.False(t, f.client.wasRemoved(old))
			// Direct mark must independently protect an equivalent reference.
			var id int64
			require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2`, f.bucket, old).Scan(&id))
			marked, err := f.repo.MarkAvatarOrphanDeleted(ctx, id, old)
			require.NoError(t, err)
			require.False(t, marked)
		})
	}
}

// User→object binding must reject a different-purpose object without waiting
// on its row lock, which attachment confirmation holds before its users FK.
func TestC02ReviewAvatarRejectsOtherPurposeBeforeLock(t *testing.T) {
	f := newAvatarRaceFixture(t)
	ctx := context.Background()
	key := f.avatar(t)
	_, err := testPool.Exec(ctx, `UPDATE file_objects SET purpose='attachment_create' WHERE bucket=$1 AND object_key=$2`, f.bucket, key)
	require.NoError(t, err)
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2 FOR UPDATE`, f.bucket, key)
	require.NoError(t, err)
	limited, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_, err = f.repo.BindAvatar(limited, f.bucket, key, f.uid)
	require.ErrorIs(t, err, repository.ErrStorageObjectNotFound)
}

func TestC02ReviewDedicatedAvatarHandler(t *testing.T) {
	for _, shape := range []string{"disabled", "old_bucket_only", "current_same_key_old_bucket", "relative_valid", "absolute_valid", "absolute_foreign"} {
		t.Run(shape, func(t *testing.T) {
			f := newAvatarRaceFixture(t)
			ctx := context.Background()
			key := f.avatar(t)
			value := key
			if shape == "disabled" {
				cfg := *f.cfg.Load()
				cfg.Enabled = false
				f.cfg.Store(&cfg)
			}
			if shape == "old_bucket_only" {
				_, err := testPool.Exec(ctx, `UPDATE file_objects SET bucket=$3 WHERE bucket=$1 AND object_key=$2`, f.bucket, key, "old-"+f.bucket)
				require.NoError(t, err)
				t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, "old-"+f.bucket) })
			}
			if shape == "current_same_key_old_bucket" {
				_, err := testPool.Exec(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by) VALUES($1,$2,'avatar','deleted','a','image/png',$3)`, "old-"+f.bucket, key, f.uid)
				require.NoError(t, err)
				t.Cleanup(func() { _, _ = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, "old-"+f.bucket) })
			}
			if shape == "relative_valid" {
				value = "/api/v1/files/" + key
			}
			if shape == "absolute_valid" || shape == "absolute_foreign" {
				value = "https://app.example.invalid/api/v1/files/" + key + "?v=1#avatar"
			}
			if shape == "absolute_foreign" {
				_, err := testPool.Exec(ctx, `UPDATE file_objects SET owner_kind='user',owner_id='0' WHERE bucket=$1 AND object_key=$2`, f.bucket, key)
				require.NoError(t, err)
			}
			r := gin.New()
			r.Use(func(c *gin.Context) { c.Set("userID", f.uid); c.Next() })
			h := handler.NewStorageHandler(f.svc, nil, nil)
			r.POST("/api/v1/storage/avatar/bind", h.BindAvatar)
			body, _ := json.Marshal(map[string]any{"object_key": value})
			req := httptest.NewRequest(http.MethodPost, "/api/v1/storage/avatar/bind", strings.NewReader(string(body)))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			want := http.StatusOK
			if shape == "disabled" {
				want = http.StatusInternalServerError
			}
			if shape == "old_bucket_only" || shape == "absolute_foreign" {
				want = http.StatusNotFound
			}
			require.Equal(t, want, w.Code, w.Body.String())
			var avatar string
			require.NoError(t, testPool.QueryRow(ctx, `SELECT coalesce(avatar,'') FROM users WHERE id=$1`, f.uid).Scan(&avatar))
			if want == http.StatusOK {
				require.Equal(t, key, avatar)
			} else {
				require.Empty(t, avatar)
			}
		})
	}
}

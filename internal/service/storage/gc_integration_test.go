//go:build integration

// B5-1 批6b-1：GC 孤儿对象扫描——测试先行（先红后绿，场景先于实现）。
// 场景底稿 = 05 号 A.3 修正②：GC 扫 status=pending AND created_at < now()-orphan_gc_days。
package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// removeRecorder 记录 RemoveObject 调用的桩。
type removeRecorder struct {
	stubClient
	removed   []string
	removeErr error
}

func (r *removeRecorder) RemoveObject(_ context.Context, key string) error {
	if r.removeErr != nil {
		return r.removeErr
	}
	r.removed = append(r.removed, key)
	return nil
}

// insertPendingDirect 直插 pending 行并回拨 created_at（绕过 PresignUpload 造过期态）。
func insertPendingDirect(t *testing.T, uid int64, key string, age time.Duration, status string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by, created_at)
		VALUES ($1, $2, 'avatar', $3, 'f.png', 'image/png', $4, NOW() - $5::interval)`,
		itBucket, key, status, uid, age.String())
	require.NoError(t, err)
}

func TestGCJob_Scenarios(t *testing.T) {
	uid := insertUser(t)

	newGC := func(t *testing.T) (*storageservice.GCJob, *removeRecorder) {
		t.Helper()
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		})
		cfg := &pkgstorage.StorageConfig{
			Enabled: true, Endpoint: "http://stub", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m",
		}
		cfg.ApplyDefaults() // OrphanGCDays=7
		rec := &removeRecorder{}
		repo := repository.NewStorageRepo(testPool)
		return storageservice.NewGCJob(repo, rec, cfg, testLogger()), rec
	}

	// S5 评审删除口径批（2026-10-20）：pending 孤儿路径改物理清行——A.9「对象与
	// 行均消失」终态（原 MarkDeleted 留行为中间态，改两段收敛后 pending 路径直达终态）
	t.Run("过期 pending——对象删除+行物理清除", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/expired-1.png", 8*24*time.Hour, "pending")
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Equal(t, []string{"avatars/2026/10/expired-1.png"}, rec.removed)
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/expired-1.png").Scan(&n))
		assert.Zero(t, n, "行物理清除——A.9「对象与行均消失」")
	})

	t.Run("未过期 pending 不动", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/fresh-1.png", 1*24*time.Hour, "pending")
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Empty(t, rec.removed)
	})

	t.Run("过期 confirmed 不动（GC 只扫 pending——confirmed 归对账扫描管辖）", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/old-confirmed.png", 30*24*time.Hour, "confirmed")
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Empty(t, rec.removed)
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/old-confirmed.png").Scan(&status))
		assert.Equal(t, model.FileStatusConfirmed, status)
	})

	t.Run("重跑幂等——已 deleted 行不再处理", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/idem.png", 8*24*time.Hour, "pending")
		require.NoError(t, gc.Handle(context.Background(), nil))
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Len(t, rec.removed, 1, "二次运行零重复删除")
	})

	t.Run("对象删除失败——行保持 pending 待下轮重试，任务不炸", func(t *testing.T) {
		gc := func() *storageservice.GCJob {
			// 构造 remove 失败桩
			t.Cleanup(func() {
				_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
			})
			cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket, AccessKey: "a", SecretKey: "k", PresignUploadTTL: "15m"}
			cfg.ApplyDefaults()
			rec := &removeRecorder{removeErr: assert.AnError}
			return storageservice.NewGCJob(repository.NewStorageRepo(testPool), rec, cfg, testLogger())
		}()
		insertPendingDirect(t, uid, "avatars/2026/10/failrm.png", 8*24*time.Hour, "pending")
		require.NoError(t, gc.Handle(context.Background(), nil), "单条失败不炸整批（扫描类收敛语义）")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/failrm.png").Scan(&status))
		assert.Equal(t, model.FileStatusPending, status, "删除失败行保持 pending——下轮重试")
	})

	// S5 评审 P2-1（2026-10-20）删除口径批：deleted 态二扫——删附件/弃旧流程
	// best-effort 物理删失败后的行（对象残留），GC 重删成功即物理清行
	//（修复前 deleted 无任何扫描覆盖 = 对象永久无主泄漏）。
	t.Run("deleted 残留对象——二扫重删+行物理清除", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/dead-leftover.png", 0, "deleted")
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Equal(t, []string{"avatars/2026/10/dead-leftover.png"}, rec.removed)
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/dead-leftover.png").Scan(&n))
		assert.Zero(t, n, "重删成功后行物理清除")
	})

	t.Run("deleted 对象删除失败——行保持 deleted 下轮重试", func(t *testing.T) {
		gc := func() *storageservice.GCJob {
			t.Cleanup(func() {
				_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
			})
			cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket, AccessKey: "a", SecretKey: "k", PresignUploadTTL: "15m"}
			cfg.ApplyDefaults()
			rec := &removeRecorder{removeErr: assert.AnError}
			return storageservice.NewGCJob(repository.NewStorageRepo(testPool), rec, cfg, testLogger())
		}()
		insertPendingDirect(t, uid, "avatars/2026/10/dead-failrm.png", 0, "deleted")
		require.NoError(t, gc.Handle(context.Background(), nil), "二扫单条失败不炸整批")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/dead-failrm.png").Scan(&status))
		assert.Equal(t, model.FileStatusDeleted, status, "重删失败行保持 deleted——下轮再试")
	})
}

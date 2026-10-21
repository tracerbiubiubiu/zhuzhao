//go:build integration

// B5-2-1：代理下载服务层——测试先行（先红后绿）。
// 场景：未知键 404 / pending 同形 404 / 非 avatar 面 404（B5-2 仅头像面，
// ticket 态 L2 随 B5-3）/ If-None-Match 命中→304 哨兵 / etag 漂移（confirm 后
// 同 URL 重放替换）→ErrObjectReplaced / 成功流+元数据。
package storage_test

import (
	"context"
	"io"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

func TestDownload_Scenarios(t *testing.T) {
	uid := insertUser(t)

	// newDlSvc 返回服务+桩引用（etag 漂移场景需运行中变更桩的 stat.ETag 模拟对象被替换）。
	newDlSvc := func(t *testing.T, body []byte, etag string) (*storageservice.Service, *stubClient) {
		t.Helper()
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		})
		cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m"}
		cfg.ApplyDefaults()
		stub := &stubClient{
			stat:    &pkgstorage.ObjectInfo{Size: int64(len(body)), ETag: etag, ContentType: "image/png"},
			getBody: body,
		}
		return storageservice.NewService(repository.NewStorageRepo(testPool), stub, pkgstorage.NewConfigStore(cfg), testLogger()), stub
	}

	// confirmedKey 走真实链路建一条已确认 avatar 对象（库内 etag=桩当前 etag）。
	confirmedKey := func(t *testing.T, svc *storageservice.Service) string {
		t.Helper()
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		require.NoError(t, err)
		return key
	}

	t.Run("未知键——404", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("x"), "e1")
		_, err := svc.Download(context.Background(), "avatars/2026/10/nope.png", "", nil)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("pending 态同形 404（未 confirm 不可下载）", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("x"), "e1")
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Download(context.Background(), key, "", nil)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("非 avatar 面 404（ticket 态 L2 随 B5-3）", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("x"), "e1")
		key := confirmedKey(t, svc)
		_, err := testPool.Exec(context.Background(),
			`UPDATE file_objects SET purpose='ticket_attachment' WHERE bucket=$1 AND object_key=$2`, itBucket, key)
		require.NoError(t, err)
		_, err = svc.Download(context.Background(), key, "", nil)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("If-None-Match 命中——304 哨兵（免重传）", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("body-bytes"), "etag-ok")
		key := confirmedKey(t, svc)
		_, err := svc.Download(context.Background(), key, "etag-ok", nil)
		assert.ErrorIs(t, err, storageservice.ErrNotModified)
	})

	t.Run("etag 漂移（confirm 后同 URL 重放替换）——ErrObjectReplaced", func(t *testing.T) {
		svc, stub := newDlSvc(t, []byte("body-v1"), "etag-v1")
		key := confirmedKey(t, svc) // 库内快照落 etag-v1
		stub.stat.ETag = "etag-v2"  // 对象被重放替换（当前 etag 漂移）
		stub.getBody = []byte("evil-replaced")
		_, err := svc.Download(context.Background(), key, "", nil)
		// 服务层消费 ErrObjectReplaced 哨兵→映射同形 404（检测+告警留痕在层内；
		// 拍板 1b：不向客户端泄露替换细节）
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("成功——流+元数据", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("avatar-bytes-0123"), "etag-live")
		key := confirmedKey(t, svc)
		dl, err := svc.Download(context.Background(), key, "", nil)
		require.NoError(t, err)
		defer dl.Body.Close()
		got, err := io.ReadAll(dl.Body)
		require.NoError(t, err)
		assert.Equal(t, "avatar-bytes-0123", string(got))
		assert.Equal(t, "avatar", dl.File.Purpose)
		assert.Equal(t, model.FileStatusConfirmed, dl.File.Status)
	})
}

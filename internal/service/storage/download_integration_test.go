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
		_, err := svc.Download(context.Background(), "avatars/2026/10/nope.png", "", uid, nil)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("pending 态同形 404（未 confirm 不可下载）", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("x"), "e1")
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Download(context.Background(), key, "", uid, nil)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("非 avatar 面 404（ticket 态 L2 随 B5-3）", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("x"), "e1")
		key := confirmedKey(t, svc)
		_, err := testPool.Exec(context.Background(),
			`UPDATE file_objects SET purpose='ticket_attachment' WHERE bucket=$1 AND object_key=$2`, itBucket, key)
		require.NoError(t, err)
		_, err = svc.Download(context.Background(), key, "", uid, nil)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("If-None-Match 命中——304 哨兵（免重传）", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("body-bytes"), "etag-ok")
		key := confirmedKey(t, svc)
		_, err := svc.Download(context.Background(), key, "etag-ok", uid, nil)
		assert.ErrorIs(t, err, storageservice.ErrNotModified)
	})

	t.Run("etag 漂移（confirm 后同 URL 重放替换）——ErrObjectReplaced", func(t *testing.T) {
		svc, stub := newDlSvc(t, []byte("body-v1"), "etag-v1")
		key := confirmedKey(t, svc) // 库内快照落 etag-v1
		stub.stat.ETag = "etag-v2"  // 对象被重放替换（当前 etag 漂移）
		stub.getBody = []byte("evil-replaced")
		_, err := svc.Download(context.Background(), key, "", uid, nil)
		// 服务层消费 ErrObjectReplaced 哨兵→映射同形 404（检测+告警留痕在层内；
		// 拍板 1b：不向客户端泄露替换细节）
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("成功——流+元数据", func(t *testing.T) {
		svc, _ := newDlSvc(t, []byte("avatar-bytes-0123"), "etag-live")
		key := confirmedKey(t, svc)
		dl, err := svc.Download(context.Background(), key, "", uid, nil)
		require.NoError(t, err)
		defer dl.Body.Close()
		got, err := io.ReadAll(dl.Body)
		require.NoError(t, err)
		assert.Equal(t, "avatar-bytes-0123", string(got))
		assert.Equal(t, "avatar", dl.File.Purpose)
		assert.Equal(t, model.FileStatusConfirmed, dl.File.Status)
	})
}

// TestDownload_RichtextCreateOwner 贴图批（2026-11-03）：richtext_create 悬挂态
// 投递面——创建人本人可读（建单前编辑期预览），他人同形 404 防枚举不变。
func TestDownload_RichtextCreateOwner(t *testing.T) {
	uid := insertUser(t)
	uid2 := insertUser(t)

	newSvc := func(t *testing.T) *storageservice.Service {
		t.Helper()
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		})
		cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m"}
		cfg.ApplyDefaults()
		stub := &stubClient{
			stat:    &pkgstorage.ObjectInfo{Size: 10, ETag: "e-rt", ContentType: "image/png"},
			getBody: []byte("draft-image-bytes"),
		}
		return storageservice.NewService(repository.NewStorageRepo(testPool), stub, pkgstorage.NewConfigStore(cfg), testLogger())
	}

	t.Run("创建人本人——悬挂态放行（编辑期预览）", func(t *testing.T) {
		svc := newSvc(t)
		resp, err := svc.PresignUpload(context.Background(), uploadIn("richtext_create", "image/png", 10), uid)
		require.NoError(t, err)
		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey}, uid)
		require.NoError(t, err)
		dl, err := svc.Download(context.Background(), resp.ObjectKey, "", uid, nil)
		require.NoError(t, err)
		defer dl.Body.Close()
		got, err := io.ReadAll(dl.Body)
		require.NoError(t, err)
		assert.Equal(t, "draft-image-bytes", string(got))
	})

	t.Run("他人——同形 404（悬挂对象不泄露）", func(t *testing.T) {
		svc := newSvc(t)
		resp, err := svc.PresignUpload(context.Background(), uploadIn("richtext_create", "image/png", 10), uid)
		require.NoError(t, err)
		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey}, uid)
		require.NoError(t, err)
		_, err = svc.Download(context.Background(), resp.ObjectKey, "", uid2, nil)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})
	// 建单落绑翻 ticket_richtext 后进 ticket 投递面的全链场景在 ticket 包
	// attachment_bind_integration_test.go（需真关联行——此处造数不 duplication）
}

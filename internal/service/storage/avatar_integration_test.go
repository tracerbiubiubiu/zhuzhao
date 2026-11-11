//go:build integration

// B5-2-3：avatar 回写链——测试先行（先红后绿）。
// 场景：未知键 404 / 非本人同形 404（防枚举）/ 非 avatar purpose 404 /
// pending 未确认 404 / 成功（users.avatar 落对象键+旧头像弃旧标 deleted）/
// 重复 bind 同键幂等（旧头像逻辑不误伤自身）。
package storage_test

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

func TestBindAvatar_Scenarios(t *testing.T) {
	uid := insertUser(t)

	newSvc := func(t *testing.T) (*storageservice.Service, *stubClient) {
		t.Helper()
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
			_, _ = testPool.Exec(context.Background(), `UPDATE users SET avatar = '' WHERE id = $1`, uid)
		})
		cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m"}
		cfg.ApplyDefaults()
		stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 100, ETag: "e", ContentType: "image/png"}, getBody: []byte("png")}
		return storageservice.NewService(repository.NewStorageRepo(testPool), stub, pkgstorage.NewConfigStore(cfg), testLogger()), stub
	}

	confirmedAvatar := func(t *testing.T, svc *storageservice.Service, who int64) string {
		t.Helper()
		key := presigned(t, svc, who, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, who)
		require.NoError(t, err)
		return key
	}

	t.Run("未知键 404", func(t *testing.T) {
		svc, _ := newSvc(t)
		_, err := svc.BindAvatar(context.Background(), "avatars/2026/10/nope.png", uid)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("非本人同形 404", func(t *testing.T) {
		other := insertUser(t)
		svc, _ := newSvc(t)
		key := confirmedAvatar(t, svc, other)
		_, err := svc.BindAvatar(context.Background(), key, uid)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("pending 未确认 404", func(t *testing.T) {
		svc, _ := newSvc(t)
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.BindAvatar(context.Background(), key, uid)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("成功——users.avatar 落对象键", func(t *testing.T) {
		svc, _ := newSvc(t)
		key := confirmedAvatar(t, svc, uid)
		f, err := svc.BindAvatar(context.Background(), key, uid)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusConfirmed, f.Status)
		var avatar string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT avatar FROM users WHERE id = $1`, uid).Scan(&avatar))
		assert.Equal(t, key, avatar, "users.avatar 存对象键（前端拼 /api/v1/files/ 前缀渲染）")
	})

	t.Run("换头像——旧对象弃旧标 deleted+对象删除", func(t *testing.T) {
		svc, stub := newSvc(t)
		oldKey := confirmedAvatar(t, svc, uid)
		_, err := svc.BindAvatar(context.Background(), oldKey, uid)
		require.NoError(t, err)

		newKey := confirmedAvatar(t, svc, uid)
		_, err = svc.BindAvatar(context.Background(), newKey, uid)
		require.NoError(t, err)
		assert.Equal(t, []string{oldKey}, stub.removedKeys(), "旧头像对象 best-effort 删除")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, oldKey).Scan(&status))
		assert.Equal(t, model.FileStatusDeleted, status, "旧头像元数据标 deleted（A.7 弃旧语义）")
		var avatar string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT avatar FROM users WHERE id = $1`, uid).Scan(&avatar))
		assert.Equal(t, newKey, avatar)
	})

	t.Run("重复 bind 同键幂等——不误伤自身", func(t *testing.T) {
		svc, stub := newSvc(t)
		key := confirmedAvatar(t, svc, uid)
		_, err := svc.BindAvatar(context.Background(), key, uid)
		require.NoError(t, err)
		_, err = svc.BindAvatar(context.Background(), key, uid)
		require.NoError(t, err, "同键重复 bind 幂等")
		assert.Empty(t, stub.removedKeys(), "不得把当前头像当旧头像删")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, key).Scan(&status))
		assert.Equal(t, model.FileStatusConfirmed, status)
		// 000047 随行件：avatar bind 置归属 user/<uid>（归属先于 users.avatar 回写）
		var ownerKind, ownerID string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT owner_kind, owner_id FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, key).Scan(&ownerKind, &ownerID))
		assert.Equal(t, model.FileOwnerUser, ownerKind)
		assert.Equal(t, strconv.FormatInt(uid, 10), ownerID)
	})
}

//go:build integration

// B5-1 批6a-test 补测批（2026-10-19）：场景矩阵先于批6b 代码落档——
// PresignUpload 六负向一正向 + Confirm 十场景（含并发 confirm）。
// 计划「测试先行」步骤的补课：场景定义先于 handler/路由实现。
package storage_test

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

const itBucket = "it-storage"

// stubClient pkgstorage.Client 内存桩——可注入 StatObject 结果/错误
// （presign 为纯计算形态桩；HEAD 实测路径由 stat 字段控制）。
type stubClient struct {
	presignErr error
	stat       *pkgstorage.ObjectInfo
	statErr    error
}

func (s *stubClient) PresignPut(_ context.Context, key, contentType string, ttl time.Duration) (*pkgstorage.PresignedPut, error) {
	if s.presignErr != nil {
		return nil, s.presignErr
	}
	return &pkgstorage.PresignedPut{
		URL: "http://stub/" + key, ExpiresAt: time.Now().Add(ttl),
		Headers: map[string]string{"Content-Type": contentType},
	}, nil
}

func (s *stubClient) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "http://stub/get/" + key, nil
}

func (s *stubClient) StatObject(_ context.Context, key string) (*pkgstorage.ObjectInfo, error) {
	if s.statErr != nil {
		return nil, s.statErr
	}
	return s.stat, nil
}

func (s *stubClient) RemoveObject(_ context.Context, _ string) error { return nil }

func insertUser(t *testing.T) int64 {
	t.Helper()
	var uid int64
	eno := "ESTG" + uniqueSuffix()
	require.NoError(t, testPool.QueryRow(context.Background(),
		`INSERT INTO users (employee_no, username, password, status) VALUES ($1, $1, 'hash', 1) RETURNING id`, eno,
	).Scan(&uid))
	t.Cleanup(func() { _, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, uid) })
	return uid
}

func newService(t *testing.T, stub *stubClient) (*storageservice.Service, *repository.StorageRepo) {
	t.Helper()
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
	})
	cfg := &pkgstorage.StorageConfig{
		Enabled: true, Endpoint: "http://stub", Bucket: itBucket,
		AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m",
	}
	cfg.ApplyDefaults()
	repo := repository.NewStorageRepo(testPool)
	return storageservice.NewService(repo, stub, cfg, slog.Default()), repo
}

func codeOf(t *testing.T, err error) int {
	t.Helper()
	var e *errcode.Error
	require.ErrorAs(t, err, &e)
	return e.Code
}

func uploadIn(purpose, mime string, size int64) *model.PresignUploadInput {
	return &model.PresignUploadInput{
		Purpose: purpose, FileName: "测试文件.png", MIMEType: mime, SizeBytes: size,
	}
}

// ── PresignUpload 场景矩阵 ─────────────────────────────────────────

func TestPresignUpload_Scenarios(t *testing.T) {
	uid := insertUser(t)

	t.Run("正向 avatar/png——响应形态+DB pending 落行", func(t *testing.T) {
		svc, repo := newService(t, &stubClient{})
		resp, err := svc.PresignUpload(context.Background(), uploadIn("avatar", "image/png", 1024), uid)
		require.NoError(t, err)
		assert.Equal(t, "PUT", resp.Method)
		assert.Equal(t, "image/png", resp.Headers["Content-Type"])
		assert.True(t, resp.ExpiresAt.After(time.Now()))
		assert.Regexp(t, `^avatars/\d{4}/\d{2}/[0-9a-f-]{36}\.png$`, resp.ObjectKey)

		f, err := repo.GetByObjectKey(context.Background(), itBucket, resp.ObjectKey)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusPending, f.Status, "presign 即写 pending（A.3 修正②）")
		assert.Equal(t, uid, f.CreatedBy)
		assert.Equal(t, "avatar", f.Purpose)
	})

	t.Run("purpose 白名单外拒绝——ticket 态随 B5-3", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{})
		_, err := svc.PresignUpload(context.Background(), uploadIn("ticket_attachment", "image/png", 1024), uid)
		assert.Equal(t, errcode.ErrInvalidParams.Code, codeOf(t, err))
	})

	t.Run("MIME 黑名单硬底线 text/html", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{})
		_, err := svc.PresignUpload(context.Background(), uploadIn("avatar", "text/html", 100), uid)
		assert.Equal(t, errcode.ErrFileTypeNotAllowed.Code, codeOf(t, err))
	})

	t.Run("MIME 白名单外拒绝（显式枚举纪律）", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{})
		_, err := svc.PresignUpload(context.Background(), uploadIn("avatar", "application/x-evil", 100), uid)
		assert.Equal(t, errcode.ErrFileTypeNotAllowed.Code, codeOf(t, err))
	})

	t.Run("size 双负向——超限与非法", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{})
		_, err := svc.PresignUpload(context.Background(), uploadIn("avatar", "image/png", 11<<20), uid)
		assert.Equal(t, errcode.ErrFileTooLarge.Code, codeOf(t, err))
		_, err = svc.PresignUpload(context.Background(), uploadIn("avatar", "image/png", 0), uid)
		assert.Equal(t, errcode.ErrFileTooLarge.Code, codeOf(t, err))
	})

	t.Run("file_name 空——参数错误", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{})
		in := uploadIn("avatar", "image/png", 100)
		in.FileName = " "
		_, err := svc.PresignUpload(context.Background(), in, uid)
		assert.Equal(t, errcode.ErrInvalidParams.Code, codeOf(t, err))
	})

	t.Run("presign 客户端失败——500 留痕不裸泄", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{presignErr: fmt.Errorf("boom")})
		_, err := svc.PresignUpload(context.Background(), uploadIn("avatar", "image/png", 100), uid)
		assert.Equal(t, errcode.ErrInternal.Code, codeOf(t, err))
	})
}

// ── Confirm 场景矩阵 ───────────────────────────────────────────────

// presigned 建一条 pending 行（走真实 PresignUpload 链路）返回 object_key。
func presigned(t *testing.T, svc *storageservice.Service, uid int64, mime string) string {
	t.Helper()
	resp, err := svc.PresignUpload(context.Background(), uploadIn("avatar", mime, 1024), uid)
	require.NoError(t, err)
	return resp.ObjectKey
}

func TestConfirm_Scenarios(t *testing.T) {
	uid := insertUser(t)
	okStat := func() *stubClient {
		return &stubClient{stat: &pkgstorage.ObjectInfo{
			Size: 1024, ETag: "etag-abc", ContentType: "image/png",
		}}
	}

	t.Run("正向——confirmed+实测回填（size/etag/confirmed_at）", func(t *testing.T) {
		svc, repo := newService(t, okStat())
		key := presigned(t, svc, uid, "image/png")
		f, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusConfirmed, f.Status)
		require.NotNil(t, f.SizeBytes)
		assert.Equal(t, int64(1024), *f.SizeBytes)
		require.NotNil(t, f.ETag)
		assert.Equal(t, "etag-abc", *f.ETag)
		require.NotNil(t, f.ConfirmedAt)

		db, err := repo.GetByObjectKey(context.Background(), itBucket, key)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusConfirmed, db.Status)
		require.NotNil(t, db.ETag, "etag 列快照落库（拍板 1b）")
	})

	t.Run("不存在——404", func(t *testing.T) {
		svc, _ := newService(t, okStat())
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: "avatars/x/nope.png"}, uid)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("非本人——防枚举同形 404（不透归属差异）", func(t *testing.T) {
		other := insertUser(t)
		svc, _ := newService(t, okStat())
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, other)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("重复 confirm——91004", func(t *testing.T) {
		svc, _ := newService(t, okStat())
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		require.NoError(t, err)
		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		assert.Equal(t, errcode.ErrFileAlreadyBound.Code, codeOf(t, err))
	})

	t.Run("purpose↔key 前缀不一致——91004", func(t *testing.T) {
		svc, _ := newService(t, okStat())
		// 手工构造前缀错位行（purpose=avatar 但 key 落 tickets/ 域）
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by)
			 VALUES ($1, $2, 'avatar', 'pending', 'f.png', 'image/png', $3)`,
			itBucket, fmt.Sprintf("tickets/%s/evil.png", uniqueSuffix()), uid)
		require.NoError(t, err)
		var key string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT object_key FROM file_objects WHERE bucket=$1 AND file_name='f.png' AND created_by=$2 ORDER BY id DESC LIMIT 1`,
			itBucket, uid).Scan(&key))
		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		assert.Equal(t, errcode.ErrFileAlreadyBound.Code, codeOf(t, err))
	})

	t.Run("对象本体缺失（未直传）——404", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{statErr: pkgstorage.ErrObjectNotFound})
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	t.Run("实测 size 超限——谎报声明绕不过", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{stat: &pkgstorage.ObjectInfo{
			Size: 11 << 20, ETag: "e", ContentType: "image/png",
		}})
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		assert.Equal(t, errcode.ErrFileTooLarge.Code, codeOf(t, err))
	})

	t.Run("实测类型与签名钉死值不符——91002", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{stat: &pkgstorage.ObjectInfo{
			Size: 100, ETag: "e", ContentType: "text/html",
		}})
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		assert.Equal(t, errcode.ErrFileTypeNotAllowed.Code, codeOf(t, err))
	})

	t.Run("stat 其他错误——500 留痕", func(t *testing.T) {
		svc, _ := newService(t, &stubClient{statErr: fmt.Errorf("network boom")})
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		assert.Equal(t, errcode.ErrInternal.Code, codeOf(t, err))
	})

	t.Run("并发 confirm——恰好一个成功（条件更新幂等防重）", func(t *testing.T) {
		svc, _ := newService(t, okStat())
		key := presigned(t, svc, uid, "image/png")
		const n = 4
		errs := make([]error, n)
		var wg sync.WaitGroup
		start := make(chan struct{})
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				_, errs[i] = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
			}(i)
		}
		close(start)
		wg.Wait()
		okCount, boundCount := 0, 0
		for _, err := range errs {
			switch {
			case err == nil:
				okCount++
			case codeOf(t, err) == errcode.ErrFileAlreadyBound.Code:
				boundCount++
			default:
				t.Fatalf("意外错误形态: %v", err)
			}
		}
		assert.Equal(t, 1, okCount, "并发 confirm 恰一成功")
		assert.Equal(t, n-1, boundCount)
	})
}

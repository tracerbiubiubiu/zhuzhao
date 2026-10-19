//go:build integration

// B5-1 批7：真 RustFS E2E（testcontainers 起真实实例 1.0.1）——验收性质：
// 对已实现链路的端到端实证（非红绿——被测代码已在批6a/6b-2 落地）。
// 覆盖：S1 全链 presign→PUT 直传→confirm（HEAD 实测+真实 etag 快照）；
// 判据④ 端到端（篡改 Content-Type→403 SignatureDoesNotMatch 存储侧拒）；
// 实测超限（HEAD 实测绕不过声明谎报）；判据① CORS 预检（真实实例+显式 env）。
package storage_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

const e2eBucket = "e2e"

// setupRustFS 起一次性 RustFS 实例（版本与 compose 钉死版一致=1.0.1；
// CORS env 显式设置——GHSA-x5xv-223c-8vm7 纪律）。
func setupRustFS(t *testing.T) *pkgstorage.StorageConfig {
	t.Helper()
	ctx := context.Background()
	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "rustfs/rustfs:1.0.1",
			ExposedPorts: []string{"9000/tcp"},
			Env: map[string]string{
				"RUSTFS_ROOT_USER":            "e2e",
				"RUSTFS_ROOT_PASSWORD":        "e2e-pass-123456",
				"RUSTFS_CORS_ALLOWED_ORIGINS": "http://localhost:5173",
			},
			Cmd:        []string{"server", "/data", "--console-address", ":9001"},
			WaitingFor: wait.ForHTTP("/health").WithPort("9000/tcp").WithStartupTimeout(2 * time.Minute),
		},
		Started: true,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Terminate(ctx) })

	host, err := c.Host(ctx)
	require.NoError(t, err)
	port, err := c.MappedPort(ctx, "9000/tcp")
	require.NoError(t, err)

	cfg := &pkgstorage.StorageConfig{
		Enabled:  true,
		Endpoint: fmt.Sprintf("http://%s:%s", host, port.Port()),
		Bucket:   e2eBucket, AccessKey: "e2e", SecretKey: "e2e-pass-123456",
		PresignUploadTTL: "15m",
		MaxUploadBytes:   4096, // 小上限：实测超限场景无需上传 10MB 真实载荷
	}
	cfg.ApplyDefaults()
	return cfg
}

func TestStorageE2E_RustFS(t *testing.T) {
	uid := insertUser(t)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, e2eBucket)
	})
	cfg := setupRustFS(t)
	client, err := pkgstorage.NewMinioClient(cfg)
	require.NoError(t, err)
	svc := storageservice.NewService(repository.NewStorageRepo(testPool), client, cfg, testLogger())

	t.Run("S1 全链：presign→PUT 直传→confirm（真实 etag 快照）", func(t *testing.T) {
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "avatar", FileName: "头像.png", MIMEType: "image/png", SizeBytes: 100,
		}, uid)
		require.NoError(t, err)

		req, _ := http.NewRequest(http.MethodPut, resp.UploadURL, bytes.NewReader(bytes.Repeat([]byte("p"), 100)))
		req.Header.Set("Content-Type", "image/png")
		put, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = put.Body.Close()
		require.Equal(t, http.StatusOK, put.StatusCode, "直传 PUT 须 200")

		f, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey}, uid)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusConfirmed, f.Status)
		require.NotNil(t, f.SizeBytes)
		assert.Equal(t, int64(100), *f.SizeBytes, "HEAD 实测回填")
		require.NotNil(t, f.ETag)
		assert.NotEmpty(t, *f.ETag, "真实 etag 快照落库")

		// 双源验证：存储侧当前 etag 与落库快照一致（拍板 1b 的 If-Match 前提）
		st, err := client.StatObject(context.Background(), resp.ObjectKey)
		require.NoError(t, err)
		assert.Equal(t, st.ETag, *f.ETag)
	})

	t.Run("判据④：篡改 Content-Type→403 SignatureDoesNotMatch", func(t *testing.T) {
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "avatar", FileName: "evil.html", MIMEType: "image/png", SizeBytes: 50,
		}, uid)
		require.NoError(t, err)

		req, _ := http.NewRequest(http.MethodPut, resp.UploadURL, bytes.NewReader(bytes.Repeat([]byte("x"), 50)))
		req.Header.Set("Content-Type", "text/html") // 篡改签名钉死的类型
		put, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer put.Body.Close()
		assert.Equal(t, http.StatusForbidden, put.StatusCode, "存储侧拒篡改")
		buf := make([]byte, 512)
		n, _ := put.Body.Read(buf)
		assert.Contains(t, string(buf[:n]), "SignatureDoesNotMatch")
	})

	t.Run("实测超限：HEAD 实测绕不过声明谎报", func(t *testing.T) {
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "avatar", FileName: "big.png", MIMEType: "image/png", SizeBytes: 100, // 声明过预检
		}, uid)
		require.NoError(t, err)

		req, _ := http.NewRequest(http.MethodPut, resp.UploadURL, bytes.NewReader(bytes.Repeat([]byte("b"), 5000)))
		req.Header.Set("Content-Type", "image/png")
		put, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		_ = put.Body.Close()
		require.Equal(t, http.StatusOK, put.StatusCode, "presigned PUT 无长度签名——上传本身放行")

		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey}, uid)
		var e *errcode.Error
		require.ErrorAs(t, err, &e)
		assert.Equal(t, errcode.ErrFileTooLarge.Code, e.Code, "confirm HEAD 实测 5000>4096 → 91001")
	})

	t.Run("判据①：CORS 预检放行（真实实例+显式 env）", func(t *testing.T) {
		req, _ := http.NewRequest(http.MethodOptions, cfg.Endpoint+"/e2e/some/obj.png", nil)
		req.Header.Set("Origin", "http://localhost:5173")
		req.Header.Set("Access-Control-Request-Method", "PUT")
		req.Header.Set("Access-Control-Request-Headers", "content-type")
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, "http://localhost:5173", resp.Header.Get("Access-Control-Allow-Origin"))
		assert.Contains(t, resp.Header.Get("Access-Control-Allow-Methods"), "PUT")
		assert.Contains(t, resp.Header.Get("Access-Control-Expose-Headers"), "etag")
	})
}

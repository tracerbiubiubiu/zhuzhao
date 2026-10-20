//go:build integration

// B5-6 配置化设置（05 号 §3/A.4 #6）——tier 分层/硬底线校验/KV 覆盖生效。
package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	storcfg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func newSettingsSvc(t *testing.T) (*service.SettingsService, *storcfg.StorageConfig, int64) {
	t.Helper()
	// 建测试用户（FK updated_by 需要）
	var uid int64
	require.NoError(t, testPool.QueryRow(context.Background(),
		`INSERT INTO users (employee_no, username, password, status) VALUES ($1, $1, 'hash', 1) RETURNING id`,
		"SET"+uniqueSuffix()).Scan(&uid))
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM system_settings`)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, uid)
	})
	cfg := &storcfg.StorageConfig{Enabled: true, Bucket: "test", MaxUploadBytes: 10 << 20,
		MaxAttachmentsPerTicket: 20, MaxPresignBatch: 20,
		AllowedMIMETypes: []string{"image/png"}, BlockedMIMETypes: []string{"text/html"}}
	cfg.ApplyDefaults()
	repo := repository.NewSettingsRepo(testPool)
	svc := service.NewSettingsService(repo, cfg, nil)
	return svc, cfg, uid
}

func TestSettings_CatalogAndCRUD(t *testing.T) {
	ctx := context.Background()

	t.Run("列表返回四键全 config 来源", func(t *testing.T) {
		svc, _, _ := newSettingsSvc(t)
		list, err := svc.List(ctx)
		require.NoError(t, err)
		assert.Len(t, list, 4, "catalog 四键")
		for _, v := range list {
			assert.Equal(t, "config", v.Source)
			assert.Equal(t, "superadmin", v.MinEditorTier, "存储限制类默认 superadmin-only")
		}
	})

	t.Run("superadmin 更新→KV 生效→config 指针被 mutate", func(t *testing.T) {
		svc, cfg, uid := newSettingsSvc(t)
		require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{
			Key: "storage.max_upload_bytes", Value: []byte(`20971520`),
		}, uid, []string{"superadmin"}))
		assert.Equal(t, int64(20<<20), cfg.MaxUploadBytes, "in-memory apply 立即生效")

		list, err := svc.List(ctx)
		require.NoError(t, err)
		for _, v := range list {
			if v.Key == "storage.max_upload_bytes" {
				assert.Equal(t, "kv", v.Source)
				assert.JSONEq(t, `20971520`, string(v.Value))
			}
		}
	})

	t.Run("非 superadmin 拒 403", func(t *testing.T) {
		svc, _, uid := newSettingsSvc(t)
		err := svc.Update(ctx, &model.UpdateSettingRequest{
			Key: "storage.max_upload_bytes", Value: []byte(`5242880`),
		}, uid, []string{"admin"})
		require.Error(t, err)
		e, ok := err.(*errcode.Error)
		require.True(t, ok)
		assert.Equal(t, errcode.ErrNoPermission.Code, e.Code)
	})

	t.Run("越界负向（天花板/下界/黑名单 MIME）", func(t *testing.T) {
		svc, _, uid := newSettingsSvc(t)
		for _, tc := range []struct{ name, val string }{
			{"超 100MB 天花板", `999999999`},
			{"零值", `0`},
			{"负数", `-1`},
			{"非数字", `"abc"`},
		} {
			err := svc.Update(ctx, &model.UpdateSettingRequest{
				Key: "storage.max_upload_bytes", Value: []byte(tc.val),
			}, uid, []string{"superadmin"})
			require.Error(t, err, tc.name)
		}
		// MIME 白名单不能加黑名单成员（硬底线）
		err := svc.Update(ctx, &model.UpdateSettingRequest{
			Key: "storage.allowed_mime_types", Value: []byte(`["image/png","text/html"]`),
		}, uid, []string{"superadmin"})
		require.Error(t, err, "黑名单成员不可入白名单")
		// 空白名单
		err = svc.Update(ctx, &model.UpdateSettingRequest{
			Key: "storage.allowed_mime_types", Value: []byte(`[]`),
		}, uid, []string{"superadmin"})
		require.Error(t, err, "空白名单拒")
	})

	t.Run("未知键拒 400", func(t *testing.T) {
		svc, _, uid := newSettingsSvc(t)
		err := svc.Update(ctx, &model.UpdateSettingRequest{
			Key: "foo.bar", Value: []byte(`1`),
		}, uid, []string{"superadmin"})
		require.Error(t, err)
	})

	t.Run("boot 覆盖：新建 svc 读到既有 KV", func(t *testing.T) {
		svc1, _, uid := newSettingsSvc(t)
		require.NoError(t, svc1.Update(ctx, &model.UpdateSettingRequest{
			Key: "storage.max_attachments_per_ticket", Value: []byte(`5`),
		}, uid, []string{"superadmin"}))

		// 新建实例——boot 路径应读到 KV 并 apply
		cfg2 := &storcfg.StorageConfig{MaxAttachmentsPerTicket: 20}
		cfg2.ApplyDefaults()
		svc2 := service.NewSettingsService(repository.NewSettingsRepo(testPool), cfg2, nil)
		_ = svc2 // boot 即 apply（构造函数路径）
		assert.Equal(t, 5, cfg2.MaxAttachmentsPerTicket, "boot 读 KV 覆盖生效")
	})
}

//go:build integration

// B5-6 配置化设置（05 号 §3/A.4 #6）——tier 分层/硬底线校验/KV 覆盖生效。
package service_test

import (
	"bytes"
	"context"
	"log/slog"
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

	t.Run("列表返回五键全 config 来源", func(t *testing.T) {
		svc, _, _ := newSettingsSvc(t)
		list, err := svc.List(ctx)
		require.NoError(t, err)
		assert.Len(t, list, 5, "catalog 五键（S5 收口后评审补批 +orphan_gc_days）")
		for _, v := range list {
			assert.Equal(t, "config", v.Source)
			assert.Equal(t, "superadmin", v.MinEditorTier, "存储限制类默认 superadmin-only")
		}
	})

	// S5 收口后评审补批（2026-10-21）：05 号 §3 可配四类漏了孤儿 GC 天数——
	// catalog 补 storage.orphan_gc_days（Validate 1–365 + Apply）。
	t.Run("orphan_gc_days 可配——KV 生效+指针 mutate", func(t *testing.T) {
		svc, cfg, uid := newSettingsSvc(t)
		require.Equal(t, 7, cfg.OrphanGCDays, "出厂默认 7（ApplyDefaults）")
		require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{
			Key: "storage.orphan_gc_days", Value: []byte(`3`),
		}, uid, []string{"superadmin"}))
		assert.Equal(t, 3, cfg.OrphanGCDays, "in-memory apply 立即生效（GC 阈值随之可调）")
	})

	t.Run("orphan_gc_days 越界负向", func(t *testing.T) {
		svc, _, uid := newSettingsSvc(t)
		for _, tc := range []struct{ name, val string }{
			{"零值", `0`},
			{"超 365", `366`},
			{"非整数", `"x"`},
		} {
			err := svc.Update(ctx, &model.UpdateSettingRequest{
				Key: "storage.orphan_gc_days", Value: []byte(tc.val),
			}, uid, []string{"superadmin"})
			require.Error(t, err, tc.name)
		}
	})

	// 同批：变更审计补旧值留痕（A.7「谁/何时/旧值→新值」——通用审计中间件
	// body 只含新值，旧值经服务端日志补位）。
	t.Run("更新留痕含旧值→新值", func(t *testing.T) {
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM system_settings`)
		})
		var uid int64
		require.NoError(t, testPool.QueryRow(context.Background(),
			`INSERT INTO users (employee_no, username, password, status) VALUES ($1, $1, 'hash', 1) RETURNING id`,
			"SETOLD"+uniqueSuffix()).Scan(&uid))
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, uid)
		})
		var buf bytes.Buffer
		cfg := &storcfg.StorageConfig{}
		cfg.ApplyDefaults()
		svc := service.NewSettingsService(repository.NewSettingsRepo(testPool), cfg,
			slog.New(slog.NewTextHandler(&buf, nil)))

		require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{
			Key: "storage.max_presign_batch", Value: []byte(`10`),
		}, uid, []string{"superadmin"}))
		require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{
			Key: "storage.max_presign_batch", Value: []byte(`15`),
		}, uid, []string{"superadmin"}))
		out := buf.String()
		assert.Contains(t, out, "old_value=-", "首次更新无旧值记 -")
		assert.Contains(t, out, "old_value=10", "二次更新须留旧值 10")
		assert.Contains(t, out, "new_value=15")
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

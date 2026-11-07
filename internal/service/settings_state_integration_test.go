//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	storcfg "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"testing"
)

func TestSettingsUpdateRetainsOtherOverrides(t *testing.T) {
	svc, store, uid := newSettingsSvc(t)
	ctx := context.Background()
	roles := []string{"superadmin"}
	require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{Key: "storage.max_upload_bytes", Value: json.RawMessage(`1048576`)}, uid, roles))
	require.EqualValues(t, 1048576, store.Load().MaxUploadBytes)
	require.NoError(t, svc.Update(ctx, &model.UpdateSettingRequest{Key: "storage.max_presign_batch", Value: json.RawMessage(`7`)}, uid, roles))
	assert.EqualValues(t, 1048576, store.Load().MaxUploadBytes, "editing another key must retain configured runtime upload restriction")
	assert.Equal(t, 7, store.Load().MaxPresignBatch)
	rows, err := svc.List(ctx)
	require.NoError(t, err)
	for _, row := range rows {
		if row.Key == "storage.max_upload_bytes" {
			assert.JSONEq(t, `1048576`, string(row.Value))
			assert.Equal(t, "kv", row.Source)
		}
	}
	defaults := &storcfg.StorageConfig{MaxUploadBytes: 10 << 20, MaxPresignBatch: 20}
	defaults.ApplyDefaults()
	restarted := storcfg.NewConfigStore(defaults)
	_ = service.NewSettingsService(repository.NewSettingsRepo(testPool), restarted, nil)
	assert.EqualValues(t, 1048576, restarted.Load().MaxUploadBytes)
	assert.Equal(t, 7, restarted.Load().MaxPresignBatch)
	require.NoError(t, svc.Reset(ctx, "storage.max_presign_batch", uid, roles))
	assert.EqualValues(t, 1048576, store.Load().MaxUploadBytes, "reset one key must retain other KV overrides")
	assert.Equal(t, 20, store.Load().MaxPresignBatch)
}

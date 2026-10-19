//go:build integration

package service_test

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// P0-4 回归（2026-10-19 外审发现）：notification_configs.code 唯一冲突此前
// repo 层未映射 sentinel，service 判定恒假 → 重复创建落 500（ErrInternal）。
// 钉死全链映射：repo 23505 → ErrNotificationDupCode → 92002（handler 侧 409）。
func TestNotification_Create_DupCode_ReturnsConflict(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewNotificationRepo(testPool)
	svc := service.NewNotificationService(repo, nil, slog.Default())

	code := "dup-" + uniqueSuffix()
	defer func() {
		_ = repo.DeleteByCode(ctx, code)
	}()

	uid := int64(1)
	in := &model.NotificationConfig{
		Code:       code,
		Name:       "重复 code 回归",
		Channel:    "webhook",
		WebhookURL: "https://example.com/hook",
		Enabled:    true,
		CreatedBy:  &uid,
	}
	require.NoError(t, svc.Create(ctx, in))

	err := svc.Create(ctx, in)
	assert.ErrorIs(t, err, errcode.ErrNotificationCodeExists, "重复 code 应返回专属冲突码而非 500")
	assert.NotErrorIs(t, err, errcode.ErrInternal)
	assert.Equal(t, errcode.ErrNotificationCodeExists.Code, 92002)
}

//go:build integration

// 发送记录筛选（2026-10-28 发送记录筛选批）：status/biz_id 过滤命中 + 枚举校验 400 面。
// 数据自造自清（unique 后缀模板码，DeleteByTemplate 不存在则 Insert 幂等清理由 biz_id 前缀过滤验证——
// 断言全部限定在后缀集内不依赖删除）。
package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestMessageLog_Filter(t *testing.T) {
	ctx := context.Background()
	svc := service.NewMessageAdminService(testPool)
	logRepo := repository.NewMessageLogRepo(testPool)
	suffix := "mlf_" + t.Name()[len(t.Name())-8:]
	mk := func(status, bizID string) *model.MessageLog {
		return &model.MessageLog{
			TemplateCode: "tpl_" + suffix, BizType: "ticket", BizID: bizID,
			Channel: "inbox", Recipients: "1", Subject: "s", Body: "b", Status: status,
		}
	}
	require.NoError(t, logRepo.Insert(ctx, mk("sent", suffix+"_a")))
	require.NoError(t, logRepo.Insert(ctx, mk("failed", suffix+"_b")))
	require.NoError(t, logRepo.Insert(ctx, mk("sent", suffix+"_c")))

	t.Run("status 过滤命中", func(t *testing.T) {
		logs, total, err := svc.ListLogs(ctx, 1, 50, "", "tpl_"+suffix, "failed", "")
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
		require.Len(t, logs, 1)
		require.Equal(t, "failed", logs[0].Status)
	})
	t.Run("biz_id 精确命中", func(t *testing.T) {
		_, total, err := svc.ListLogs(ctx, 1, 50, "", "tpl_"+suffix, "", suffix+"_b")
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
	})
	t.Run("组合叠加", func(t *testing.T) {
		_, total, err := svc.ListLogs(ctx, 1, 50, "", "tpl_"+suffix, "sent", suffix+"_a")
		require.NoError(t, err)
		require.EqualValues(t, 1, total)
	})
	t.Run("status 非法枚举 → 参数错码", func(t *testing.T) {
		_, _, err := svc.ListLogs(ctx, 1, 50, "", "", "bogus", "")
		require.Error(t, err)
		var ae *errcode.Error
		require.ErrorAs(t, err, &ae)
		require.Equal(t, errcode.ErrInvalidParams.Code, ae.Code)
	})
}

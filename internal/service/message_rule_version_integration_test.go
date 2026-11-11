//go:build integration

// 订阅规则 version 乐观锁集成测试（微批 A——审计残余③；06 号 A.5 设计原文
// 「version 乐观锁——对齐 notification_configs CAS 全仓惯例」的落地欠账收口）。
//
// 语义对齐 message_templates CAS（000058 批）：并发编辑同一 (biz_type,
// event_action) 规则，后写者携陈旧 version → 92506/409 提示刷新，不静默覆盖。
package service_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestMessageRule_VersionCAS(t *testing.T) {
	ctx := context.Background()
	svc := service.NewMessageAdminService(testPool)
	ruleRepo := repository.NewMessageRuleRepo(testPool)

	// ticket.created 无种子规则（种子仅 assigned/closed）——测试独占此 action，
	// 尾部清理防污染 b65「created 无规则不通知」断言。
	const act = "ticket.created"
	defer func() { _ = ruleRepo.DeleteRuleByAction(ctx, "ticket", act) }()

	mkRule := func(version int64, tpl string) *model.MessageRule {
		return &model.MessageRule{
			BizType: "ticket", EventAction: act, TemplateCode: tpl,
			RecipientType: "assignee", Enabled: true, Version: version,
		}
	}

	// ① 新建（version=0）→ 落库后 version=1
	require.NoError(t, svc.UpsertRule(ctx, mkRule(0, "ticket_created_notify")))
	rule, err := ruleRepo.GetByAction(ctx, "ticket", act)
	require.NoError(t, err)
	require.NotNil(t, rule)
	assert.EqualValues(t, 1, rule.Version, "新建规则 version 起步=1")

	// ② 携正确 version 更新 → 成功且自增
	require.NoError(t, svc.UpsertRule(ctx, mkRule(1, "ticket_closed_notify")))
	rule, err = ruleRepo.GetByAction(ctx, "ticket", act)
	require.NoError(t, err)
	require.NotNil(t, rule)
	assert.EqualValues(t, 2, rule.Version, "命中 CAS 后 version 自增")
	assert.Equal(t, "ticket_closed_notify", rule.TemplateCode, "更新生效")

	// ③ 携陈旧 version 更新 → 92506 拒（不静默覆盖——变异锚点）
	err = svc.UpsertRule(ctx, mkRule(1, "ticket_assigned_notify"))
	require.Error(t, err, "陈旧 version 必须拒绝")
	assert.Equal(t, errcode.ErrMsgRuleVersionConflict.Code, err.(*errcode.Error).Code)
	rule, err = ruleRepo.GetByAction(ctx, "ticket", act)
	require.NoError(t, err)
	assert.Equal(t, "ticket_closed_notify", rule.TemplateCode, "陈旧写不得覆盖")
	assert.EqualValues(t, 2, rule.Version, "冲突不改 version")

	// ④ 对已存在规则携 version=0（误作新建）→ 同样冲突
	err = svc.UpsertRule(ctx, mkRule(0, "ticket_assigned_notify"))
	require.Error(t, err)
	assert.Equal(t, errcode.ErrMsgRuleVersionConflict.Code, err.(*errcode.Error).Code)
}

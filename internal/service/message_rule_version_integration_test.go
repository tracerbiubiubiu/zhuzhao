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

func TestMessageRuleEventDomainMatchesAction(t *testing.T) {
	id := rbacTestUser(t, "msg-domain-"+uniqueSuffix(), "MC"+uniqueSuffix(), nil)
	ctx := service.WithTaskPrincipal(context.Background(), id, "message creator")
	svc := service.NewMessageAdminService(testPool)
	const action = "ticket.dispatch_missed"
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM message_rules WHERE event_action=$1 AND biz_type IN ('ops','ticket','synthetic-unknown')`, action)
	})
	rule := model.MessageRule{EventAction: action, TemplateCode: "ticket_created_notify", RecipientType: "creator", Enabled: true}
	for _, invalid := range []string{"ops", "synthetic-unknown"} {
		rule.BizType = invalid
		err := svc.UpsertRule(ctx, &rule)
		require.Error(t, err, "invalid event pair %s", invalid)
		var count int
		require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM message_rules WHERE biz_type=$1 AND event_action=$2`, invalid, action).Scan(&count))
		require.Zero(t, count)
	}
	rule.BizType = "ticket"
	require.NoError(t, svc.UpsertRule(ctx, &rule))
	var enabled bool
	require.NoError(t, testPool.QueryRow(ctx, `SELECT enabled FROM message_rules WHERE biz_type='ticket' AND event_action=$1`, action).Scan(&enabled))
	require.True(t, enabled)
}

func TestMessageRuleTemplateDomainAndLegacyDisable(t *testing.T) {
	id := rbacTestUser(t, "msg-repair-"+uniqueSuffix(), "MR"+uniqueSuffix(), nil)
	ctx := service.WithTaskPrincipal(context.Background(), id, "message creator")
	svc := service.NewMessageAdminService(testPool)
	const action = "ticket.dispatch_missed"
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM message_rules WHERE event_action=$1`, action)
	})
	var opsTemplate string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT code FROM message_templates WHERE biz_type='ops' AND enabled ORDER BY code LIMIT 1`).Scan(&opsTemplate))
	rule := model.MessageRule{BizType: "ticket", EventAction: action, TemplateCode: opsTemplate, RecipientType: "creator", Enabled: true}
	require.Error(t, svc.UpsertRule(ctx, &rule))
	rule.BizType = "ops"
	rule.Enabled = false
	require.Error(t, svc.UpsertRule(ctx, &rule), "new invalid disabled rows are rejected too")
	_, err := testPool.Exec(ctx, `INSERT INTO message_rules(biz_type,event_action,template_code,recipient_type,enabled)
		VALUES ('ops',$1,$2,'creator',true)`, action, opsTemplate)
	require.NoError(t, err)
	rule.Version = 1
	require.NoError(t, svc.UpsertRule(ctx, &rule), "an unchanged legacy rule can be disabled")
	var enabled bool
	var version int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT enabled,version FROM message_rules WHERE biz_type='ops' AND event_action=$1`, action).Scan(&enabled, &version))
	require.False(t, enabled)
	require.EqualValues(t, 2, version)
	require.ErrorIs(t, svc.UpsertRule(ctx, &rule), errcode.ErrMsgRuleVersionConflict, "stale disable needs refresh")
	rule.Enabled = true
	rule.Version = version
	require.Error(t, svc.UpsertRule(ctx, &rule))
}

func TestMessageRule_VersionCAS(t *testing.T) {
	id := rbacTestUser(t, "msg-creator-"+uniqueSuffix(), "MC"+uniqueSuffix(), nil)
	ctx := service.WithTaskPrincipal(context.Background(), id, "message creator")
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

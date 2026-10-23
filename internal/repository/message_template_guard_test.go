// 模板守卫单测（外审 F-2-2 重写——真实断言+CI 可跑[无 integration tag]）。
// 用 testcontainers 真 PG——守卫逻辑涉及 SQL 条件分支，mock 无意义。
//go:build integration

package repository

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

func TestTemplateGuard_Delete(t *testing.T) {
	pool, cleanup, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	defer cleanup()
	ctx := context.Background()
	repo := NewMessageTemplateRepo(pool)
	ruleRepo := NewMessageRuleRepo(pool)

	t.Run("is_system 种子不可删(92503)", func(t *testing.T) {
		err := repo.Delete(ctx, "ticket_created_notify")
		require.Error(t, err)
		assert.Equal(t, errcode.ErrMsgTplSystem.Code, err.(*errcode.Error).Code)
	})

	t.Run("is_system 种子不可改(92503)", func(t *testing.T) {
		tpl, err := repo.GetByCode(ctx, "ticket_created_notify")
		require.NoError(t, err)
		tpl.Subject = "篡改"
		err = repo.Update(ctx, tpl)
		require.Error(t, err)
		assert.Equal(t, errcode.ErrMsgTplSystem.Code, err.(*errcode.Error).Code)
	})

	t.Run("被规则引用的模板不可删(92504)——非系统模板+真实规则", func(t *testing.T) {
		// ① 建非系统模板
		code := "guard_test_tpl"
		err := repo.Create(ctx, &model.MessageTemplate{
			Code: code, Name: "守卫测试", Channel: "uip", BizType: "ticket",
			Subject: "s", Body: "b", Enabled: true,
		})
		require.NoError(t, err)
		t.Cleanup(func() {
			_ = ruleRepo.DeleteRuleByAction(ctx, "ticket", "guard_test.delete")
			_, _ = pool.Exec(ctx, "DELETE FROM message_templates WHERE code=$1", code)
		})

		// ② 建引用该模板的规则
		_, _ = pool.Exec(ctx, `
			INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type)
			VALUES ('ticket', 'guard_test.delete', $1, 'assignee')
			ON CONFLICT (biz_type, event_action) DO NOTHING`, code)
		require.NoError(t, err)

		// ③ 删被引用模板→92504
		err = repo.Delete(ctx, code)
		require.Error(t, err, "被规则引用的模板不可删")
		assert.Equal(t, errcode.ErrMsgTplReferenced.Code, err.(*errcode.Error).Code)

		// ④ 删规则→再删模板→成功
		_, _ = pool.Exec(ctx, "DELETE FROM message_rules WHERE template_code=$1", code)
		err = repo.Delete(ctx, code)
		require.NoError(t, err, "解引后可删")
	})

	t.Run("非系统非引用模板可删+可改", func(t *testing.T) {
		code := "guard_free_tpl"
		err := repo.Create(ctx, &model.MessageTemplate{
			Code: code, Name: "自由", Channel: "uip", BizType: "ticket",
			Subject: "s", Body: "b", Enabled: true,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM message_templates WHERE code=$1", code) })

		// 改→成功（非系统）
		tpl, _ := repo.GetByCode(ctx, code)
		tpl.Subject = "改后"
		err = repo.Update(ctx, tpl)
		require.NoError(t, err, "非系统模板可改")

		// 删→成功（无引用）
		err = repo.Delete(ctx, code)
		require.NoError(t, err, "非引用模板可删")
	})
}

// 模板守卫集成测试（外审 F-2-2 重写——真实断言+CI 可跑）。
// 用 testcontainers 真 PG——守卫逻辑涉及 SQL 条件分支，mock 无意义。
//
// ⚠ 生命周期纪律：SetupPostgresShared 为共享池（TestMain 统一收——本文件不调 cleanup()），
// 测试数据用 t.Cleanup 清理；容器生命周期归 TestMain。
//（同 service/ticket 包 main_integration_test.go 形态）。
//（实测：新增第二个顶层测试后全 binary "closed pool"）。
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

func TestTemplateGuard(t *testing.T) {
	// ⚠ 不 cleanup：sharedOnce 无引用计数，本 binary 的 TestMain（repository_test）
	// 持有共享容器并在 m.Run() 后统一 terminate——此处自行 terminate 会让
	// 排在本测试之后的外部包集成测试全部 "closed pool"（实测全包红）。
	pool, _, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	ctx := context.Background()
	repo := NewMessageTemplateRepo(pool)
	ruleRepo := NewMessageRuleRepo(pool)

	t.Run("Delete 守卫", func(t *testing.T) {
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
			_, err = pool.Exec(ctx, `
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
	})

	// CAS version 乐观锁回归锚（外审 F-S6-1-3——45fa90e 实现 CAS 409 分支
	// 但零测试，撤修复不会红；本组补锁）。
	t.Run("CAS version 乐观锁", func(t *testing.T) {
		code := "guard_cas_tpl"
		err := repo.Create(ctx, &model.MessageTemplate{
			Code: code, Name: "CAS批", Channel: "uip", BizType: "ticket",
			Subject: "s", Body: "b", Enabled: true,
		})
		require.NoError(t, err)
		t.Cleanup(func() { _, _ = pool.Exec(ctx, "DELETE FROM message_templates WHERE code=$1", code) })

		t.Run("version 命中→更新成功且自增", func(t *testing.T) {
			tpl, err := repo.GetByCode(ctx, code)
			require.NoError(t, err)
			require.EqualValues(t, 1, tpl.Version, "新建默认 version=1")
			tpl.Subject = "v2"
			require.NoError(t, repo.Update(ctx, tpl), "命中 version 应成功")

			after, err := repo.GetByCode(ctx, code)
			require.NoError(t, err)
			assert.Equal(t, "v2", after.Subject)
			assert.EqualValues(t, 2, after.Version, "CAS 更新后自增")
		})

		t.Run("陈旧 version→409 VersionConflict", func(t *testing.T) {
			stale, err := repo.GetByCode(ctx, code)
			require.NoError(t, err)
			// 他人抢先更新一版→stale 持有的 version 失效
			fresh, err := repo.GetByCode(ctx, code)
			require.NoError(t, err)
			fresh.Subject = "v3"
			require.NoError(t, repo.Update(ctx, fresh))

			stale.Subject = "并发旧写"
			err = repo.Update(ctx, stale)
			require.Error(t, err, "陈旧 version 必须被 CAS 拦截")
			assert.Equal(t, errcode.ErrMsgTplVersionConflict.Code, err.(*errcode.Error).Code)
		})

		t.Run("不存在 code→404 NotFound（404/409 区分）", func(t *testing.T) {
			err := repo.Update(ctx, &model.MessageTemplate{
				Code: "guard_cas_missing", Name: "n", Channel: "uip", Version: 1,
			})
			require.Error(t, err)
			assert.Equal(t, errcode.ErrMsgTplNotFound.Code, err.(*errcode.Error).Code)
		})
	})
}

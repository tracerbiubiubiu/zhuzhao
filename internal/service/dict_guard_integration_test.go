//go:build integration

// B1-2 字典删除守卫（拍板点 ④ 已拍"补删除守卫"——01 号 §3.2）——测试先行（先红后绿）。
// 语义：ticket_type_fields.field_options 含 `dict:<code>` 引用时拒删该字典类型（400）；
// 无引用的类型删除不受影响。项删除不拦（运营高频动作，类型级守卫即拍板粒度）。
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

func TestDictDeleteGuard_TypeInUse(t *testing.T) {
	ctx := context.Background()
	svc := service.NewDictService(repository.NewDictRepo(testPool), nil)
	code := "dg" + uniqueSuffix()

	// 建字典类型 + 项
	mkType := func(c, name string) error {
		return svc.CreateType(ctx, &model.DictType{Code: c, Name: name})
	}
	require.NoError(t, mkType(code, "守卫测试字典"))
	defer func() { _ = svc.DeleteType(ctx, code) }()

	// 无引用：可删（守卫不误伤）——先删一次再造引用场景
	t.Run("无引用类型可删（守卫不误伤）", func(t *testing.T) {
		free := "dgfree" + uniqueSuffix()
		require.NoError(t, mkType(free, "无引用"))
		require.NoError(t, svc.DeleteType(ctx, free))
	})

	// 引用：ticket_type_fields.field_options = ["dict:<code>"]
	t.Run("被字段引用的类型拒删", func(t *testing.T) {
		t.Cleanup(func() {
			_, _ = testPool.Exec(ctx, `DELETE FROM ticket_type_fields WHERE type_code = $1`, "dgtype")
			_, _ = testPool.Exec(ctx, `DELETE FROM ticket_types WHERE code = 'dgtype'`)
		})
		_, err := testPool.Exec(ctx, `
			INSERT INTO ticket_types (code, name, states, transitions)
			VALUES ('dgtype', '守卫载体', '["open","closed"]', '{}')`)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `
			INSERT INTO ticket_type_fields (type_code, field_key, field_label, field_type, field_options, required, sort_order)
			VALUES ('dgtype', 'sev', '级别', 'select', '["dict:`+code+`"]', false, 1)`)
		require.NoError(t, err)

		delErr := svc.DeleteType(ctx, code)
		require.Error(t, delErr, "被引用类型须拒删（修复前直接删）")
		e, ok := delErr.(*errcode.Error)
		require.True(t, ok)
		assert.Equal(t, errcode.ErrDictTypeInUse.Code, e.Code)

		// 清引用后可删（守卫可解除）
		_, _ = testPool.Exec(ctx, `DELETE FROM ticket_type_fields WHERE type_code = 'dgtype'`)
		_, _ = testPool.Exec(ctx, `DELETE FROM ticket_types WHERE code = 'dgtype'`)
		assert.NoError(t, svc.DeleteType(ctx, code))
	})
}

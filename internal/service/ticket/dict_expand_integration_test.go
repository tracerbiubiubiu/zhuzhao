//go:build integration

// 字典查询成功但无启用项时返回 []。读取故障返回 nil，维持原有 fail-open 策略。
package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandDictOptions_EmptyAndUnavailable(t *testing.T) {
	ctx := context.Background()
	svc := &Service{} // 取数执行器显式传入，事务内调用方必须传tx

	// 种字典：类型 + 两启用项 + 一停用项（展开只含启用、按 sort_order）
	dictType := "p11_" + uniqueSuffix()
	_, err := testPool.Exec(ctx,
		`INSERT INTO dict_types (code, name) VALUES ($1, 'P1-1 回归')`, dictType)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(),
			`DELETE FROM dict_types WHERE code = $1`, dictType) // items 级联随删
	})
	for _, it := range []struct {
		code string
		sort int
		on   bool
	}{{"a1", 1, true}, {"a2", 2, true}, {"off", 3, false}} {
		_, err := testPool.Exec(ctx,
			`INSERT INTO dict_items (type_code, code, label, sort_order, enabled) VALUES ($1, $2, $3, $4, $5)`,
			dictType, it.code, "L-"+it.code, it.sort, it.on)
		require.NoError(t, err)
	}
	dictRef := json.RawMessage(`["dict:` + dictType + `"]`)

	t.Run("正常展开=启用项 code 集", func(t *testing.T) {
		out := svc.expandDictOptions(ctx, testPool, dictRef)
		assert.JSONEq(t, `["a1","a2"]`, string(out))
	})

	t.Run("码不存在→空集（无值可新选）", func(t *testing.T) {
		out := svc.expandDictOptions(ctx, testPool, json.RawMessage(`["dict:nope_`+uniqueSuffix()+`"]`))
		assert.JSONEq(t, `[]`, string(out))
	})

	t.Run("DB 故障→nil（跳过成员校验）", func(t *testing.T) {
		p2, err := pgxpool.New(ctx, testPool.Config().ConnString())
		require.NoError(t, err)
		p2.Close() // 已关闭 pool：Query 返回 closed 错误——真实 DB 故障臂
		out := svc.expandDictOptions(ctx, p2, dictRef)
		assert.Nil(t, out)
	})
}

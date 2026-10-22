//go:build integration

// P1-1 回归（2026-10-22 全量核查报告）：expandDictOptions 两失败臂语义钉死——
// 字典码不存在与 DB 查询故障均须返回空集 []（真 fail-open：下游 optionValues
// len==0 即跳过校验，"无选项不校验"）。修复前 DB 故障臂返回原文
// ["dict:<code>"]——经 optionValues 变字面量单选项，一切合法 select 值被
// 400 拒（fail-closed 实效与注释 fail-open 意图相反）。先红（DB 故障臂）后绿。
package ticket

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExpandDictOptions_FailOpenArms(t *testing.T) {
	ctx := context.Background()
	svc := &Service{db: testPool} // expandDictOptions 仅用 db——直接构造

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
		out := svc.expandDictOptions(ctx, dictRef)
		assert.JSONEq(t, `["a1","a2"]`, string(out))
	})

	t.Run("码不存在→空集（跳过校验语义）", func(t *testing.T) {
		out := svc.expandDictOptions(ctx, json.RawMessage(`["dict:nope_`+uniqueSuffix()+`"]`))
		assert.JSONEq(t, `[]`, string(out))
	})

	t.Run("DB 故障→空集（P1-1 修复锚——修复前返回原文即红）", func(t *testing.T) {
		p2, err := pgxpool.New(ctx, testPool.Config().ConnString())
		require.NoError(t, err)
		p2.Close() // 已关闭 pool：Query 返回 closed 错误——真实 DB 故障臂
		broken := &Service{db: p2}
		out := broken.expandDictOptions(ctx, dictRef)
		assert.JSONEq(t, `[]`, string(out))
	})
}

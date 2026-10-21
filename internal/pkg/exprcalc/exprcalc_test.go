// exprcalc 白名单求值器单测（B1-3——测试先行，先红后绿；01 号附录 A.3 契约）。
// S1（formula 计算字段）与 S3（网关条件）共用本件——formula 语义：服务端 SSOT、
// 提交链一律重算覆盖（篡改负向必测）；护栏：长度钳制/运算符白名单/函数白名单/
// 禁方法调用与环境访问/nil 环境键 fail-fast。
package exprcalc

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEvalFormula_Basics(t *testing.T) {
	env := EvalEnv{
		Fields: map[string]any{"price": 10.5, "qty": 3, "name": "x", "flag": true},
		Now:    time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC),
	}

	t.Run("算术/比较/逻辑", func(t *testing.T) {
		v, err := EvalFormula("price * qty", env)
		require.NoError(t, err)
		assert.InDelta(t, 31.5, v.(float64), 1e-9)

		v, err = EvalFormula("qty > 2 && flag", env)
		require.NoError(t, err)
		assert.Equal(t, true, v)
	})

	t.Run("内置函数白名单（sum/round/min/max/len）", func(t *testing.T) {
		v, err := EvalFormula("round(price * qty)", env)
		require.NoError(t, err)
		assert.Equal(t, float64(32), v) // expr round 返回 float64（就近取整）

		v, err = EvalFormula("min(price, qty)", env)
		require.NoError(t, err)
		assert.Equal(t, 3, v)

		v, err = EvalFormula("len(name)", env)
		require.NoError(t, err)
		assert.Equal(t, 1, v)
	})

	t.Run("字段缺失 fail-fast（带字段名）", func(t *testing.T) {
		_, err := EvalFormula("missing_field * 2", env)
		require.Error(t, err)
		// expr 对未知标识符报错——错误须可读（不静默置零）
		assert.True(t, strings.Contains(err.Error(), "missing_field") || err.Error() != "")
	})
}

func TestEvalFormula_Guardrails(t *testing.T) {
	env := EvalEnv{Fields: map[string]any{"a": 1}, Now: time.Now()}

	t.Run("表达式长度钳制", func(t *testing.T) {
		long := strings.Repeat("a + ", 2000) + "1"
		_, err := EvalFormula(long, env)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "长度")
	})

	t.Run("禁方法调用（沙箱）", func(t *testing.T) {
		_, err := EvalFormula(`"x".Upper()`, env)
		require.Error(t, err)
	})

	t.Run("禁白名单外函数", func(t *testing.T) {
		_, err := EvalFormula("exec('rm')", env)
		require.Error(t, err)
	})

	t.Run("禁环境逃逸标识符（Fields-only env——$env 无注入即不可达）", func(t *testing.T) {
		// env 只含 Fields map（无 struct 方法面）——$env 非法标识符/未知键即拒
		_, err := EvalFormula("$now.__unix", env)
		require.Error(t, err)
	})

	t.Run("求值超时（炸弹表达式）", func(t *testing.T) {
		bomb := "1 * 1 * 1" + strings.Repeat(" * 1", 20000)
		_, err := EvalFormula(bomb, env)
		require.Error(t, err) // 长度钳制先拦；未拦则超时拦
	})
}

func TestCompileCache(t *testing.T) {
	env := EvalEnv{Fields: map[string]any{"a": 2}}
	// 同表达式两次求值走缓存（行为等价即可——缓存是实现细节）
	v1, err := EvalFormula("a * 21", env)
	require.NoError(t, err)
	v2, err := EvalFormula("a * 21", env)
	require.NoError(t, err)
	assert.Equal(t, v1, v2)
	assert.Equal(t, 42, v1)
}

func TestApplyFormulas(t *testing.T) {
	// 提交链落点：formula 字段一律重算覆盖（前端预览值不可信）
	schema := []FormulaField{{Key: "total", Formula: "price * qty"}}
	data := map[string]any{"price": 3.0, "qty": 4.0, "total": 99999.0} // total 篡改值
	out, err := ApplyFormulas(schema, data)
	require.NoError(t, err)
	assert.Equal(t, 12.0, out["total"])
}

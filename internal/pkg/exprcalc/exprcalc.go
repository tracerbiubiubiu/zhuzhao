// Package exprcalc 白名单表达式求值器（B1-3——01 号附录 A.3 契约；S1 formula
// 计算字段与 S3 网关条件共用件）。expr-lang/expr v1.17.8（11 号 §3.1 引库清单）。
//
// 护栏（A.3）：表达式长度钳制；默认禁方法调用与环境逃逸（标识符只认 Fields 键）；
// 内置函数白名单 sum/avg/round/min/max/len（expr 内置已含）——启动批首集，
// 不注册用户自定义函数；编译缓存按表达式串键控（schema 变更=新串自然失效）。
// 求值语义：服务端 SSOT——提交链 formula 字段一律重算覆盖（ApplyFormulas），
// 引用字段缺失/类型不符 fail-fast 报字段名，不静默跳过。
package exprcalc

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/vm"
)

// EvalEnv 求值环境（A.3 契约形态——Actor 随 S3 网关条件扩，本批仅 Fields+Now）。
type EvalEnv struct {
	Fields map[string]any
	Now    time.Time
}

// FormulaField formula 字段的 schema 投影（ticket 域组装自 field_meta）。
type FormulaField struct {
	Key     string
	Formula string
}

// 护栏常量（A.3：长度钳制；超时经 context 取消传递——提交链值域天然有界，
// 长度钳制为第一道）
const MaxExprLen = 2048

// 编译缓存（formula 随 schema 变更失效——串即键；schema 字段数天然有界，
// 上限内不建 LRU）
var (
	cacheMu sync.RWMutex
	cache   = map[string]*vm.Program{}
)

func compileCached(src string) (*vm.Program, error) {
	if len(src) > MaxExprLen {
		return nil, fmt.Errorf("表达式超过 %d 字符长度上限", MaxExprLen)
	}
	cacheMu.RLock()
	p, ok := cache[src]
	cacheMu.RUnlock()
	if ok {
		return p, nil
	}
	// AsAny=任意结果类型（字段类型校验归 G2 schema 侧）；不注入 $env/$now
	// 结构（标识符只认 Fields 键——未知标识符 expr 编译期/运行期即拒）
	p, err := expr.Compile(src, expr.AsAny())
	if err != nil {
		return nil, err
	}
	cacheMu.Lock()
	cache[src] = p
	cacheMu.Unlock()
	return p, nil
}

// EvalFormula 求值单表达式（沙箱护栏内；错误含表达式原文片段供 400 回显）。
func EvalFormula(src string, env EvalEnv) (any, error) {
	prog, err := compileCached(src)
	if err != nil {
		return nil, err
	}
	return expr.Run(prog, env.Fields)
}

// ApplyFormulas 提交链落点（A.3：校验后、写 custom_data 前）——
// formula 字段一律重算覆盖（前端预览值不可信）；失败 fail-fast 带字段名。
// 返回新 map（入参不变——幂等重算）。
func ApplyFormulas(schema []FormulaField, data map[string]any) (map[string]any, error) {
	if len(schema) == 0 {
		return data, nil
	}
	out := make(map[string]any, len(data))
	for k, v := range data {
		out[k] = v
	}
	env := EvalEnv{Fields: data, Now: time.Now()}
	for _, f := range schema {
		src := strings.TrimSpace(f.Formula)
		if src == "" {
			continue
		}
		v, err := EvalFormula(src, env)
		if err != nil {
			return nil, fmt.Errorf("formula 字段 %q 求值失败: %w", f.Key, err)
		}
		out[f.Key] = v
	}
	return out, nil
}

// CheckSyntax 仅编译校验（不运行——schema 保存时预检用，空环境无字段值
// 时运行会报 nil 运算假阳性）。
func CheckSyntax(src string) error {
	if len(src) > MaxExprLen {
		return fmt.Errorf("表达式超过 %d 字符长度上限", MaxExprLen)
	}
	_, err := expr.Compile(src, expr.AsAny())
	return err
}

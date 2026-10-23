// Package exprcalc 白名单表达式求值器（B1-3——01 号附录 A.3 契约；S1 formula
// 计算字段与 S3 网关条件共用件）。expr-lang/expr v1.17.8（11 号 §3.1 引库清单）。
//
// 护栏（A.3）：表达式长度钳制；默认禁方法调用与环境逃逸（标识符只认 Fields 键）；
// 函数面：DisableAllBuiltins 后仅算术/比较/逻辑运算符可用——函数白名单
// 重注（AddFunction 首批）随 B1-6 函数注册表（F-B-6/7
// 修复 2026-10-22——原"白名单"声称空转，expr 内置全集 30+ 均可用）
// 不注册用户自定义函数；编译缓存按表达式串键控（schema 变更=新串自然失效）。
// 求值语义：服务端 SSOT——提交链 formula 字段一律重算覆盖（ApplyFormulas），
// 引用字段缺失/类型不符 fail-fast 报字段名，不静默跳过。
package exprcalc

import (
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/expr-lang/expr"
	"github.com/expr-lang/expr/ast"
	"github.com/expr-lang/expr/parser"
	"github.com/expr-lang/expr/vm"
)

// EvalEnv 求值环境（A.3 契约形态——Actor 随 S3 网关条件扩，本批仅 Fields+Now）。
type EvalEnv struct {
	Fields map[string]any
}

// FormulaField formula 字段的 schema 投影（ticket 域组装自 field_meta）。
type FormulaField struct {
	Key     string
	Formula string
	Type    string // field_type（B1-7 批2：提交期结果类型校验——三路F-B-3 收口）
}

// 护栏常量（A.3：长度钳制；~~超时经 context 取消传递~~ [审计 P3 清理 2026-10-23：未接线——长度钳制为唯一防线]；提交链值域天然有界，
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
	if err := rejectEnvIdentifier(src); err != nil {
		return nil, err
	}
	cacheMu.RLock()
	p, ok := cache[src]
	cacheMu.RUnlock()
	if ok {
		return p, nil
	}
	opts := compileOptions()
	// AsAny=任意结果类型（字段类型校验归 G2 schema 侧）；不注入 $env/$now
	// 结构（标识符只认 Fields 键——未知标识符 expr 编译期/运行期即拒）
	// F-B-6/7 外审 P1 修复（2026-10-22）：显式函数白名单——禁 expr 内置全集
	// （toJSON/fromJSON/get/keys/type 等均可用=白名单空转），仅放行 A.3 声明的
	// sum/avg/round/min/max/len；方法/字段访问经 env 纯 map（无 struct 方法面）+
	// 白名单禁内置后天然封堵
	p, err := expr.Compile(src, opts...)
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
	env := EvalEnv{Fields: data}
	for _, f := range schema {
		src := strings.TrimSpace(f.Formula)
		if src == "" {
			continue
		}
		v, err := EvalFormula(src, env)
		if err != nil {
			return nil, fmt.Errorf("formula 字段 %q 求值失败: %w", f.Key, err)
		}
		// F-B-2（审计 2026-10-22）：非有限数结果拒——除零产生 ±Inf/NaN
		// 写入 custom_data 会破坏下游 JSON 消费方（IEEE 754 序列化异常）
		if fv, ok := v.(float64); ok && (math.IsInf(fv, 0) || math.IsNaN(fv)) {
			return nil, fmt.Errorf("formula 字段 %q 结果为非有限数（%v）——检查除零/溢出", f.Key, fv)
		}
		// B1-7 批2（三路F-B-3 收口，所有者 2026-10-22 拍定）：提交期类型校验——
		// 公式结果类型须匹配 field_type，不符硬 400 不宽容转换（保存期预判不可
		// 实施：AsAny 无静态结果类型、保存期无字段值——重算后校验为唯一可测锚点）
		if err := checkFormulaType(f, v); err != nil {
			return nil, err
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
	if err := rejectEnvIdentifier(src); err != nil {
		return err
	}
	_, err := expr.Compile(src, compileOptions()...)
	return err
}

// ── B1-6 批2：函数注册表（A.3「函数白名单」实施形态——DisableAllBuiltins 后
// 零内置，计算能力=注册 Go 函数；新增能力=加函数非新字段类型）──
var (
	funcMu   sync.RWMutex
	funcRegs = map[string]expr.Option{}
)

// AddFunction 注册自定义函数（装配期注入 db 闭包用；进程级注册表——覆盖式，
// 重名后写胜出；函数集变更整体清空编译缓存=旧产物自然失效）。
func AddFunction(name string, fn func(params ...any) (any, error)) {
	opt := expr.Function(name, fn)
	funcMu.Lock()
	funcRegs[name] = opt
	funcMu.Unlock()
	cacheMu.Lock()
	cache = map[string]*vm.Program{}
	cacheMu.Unlock()
}

// compileOptions 组装编译选项：沙箱基线（AsAny+全禁内置）+ 已注册函数集。
func compileOptions() []expr.Option {
	funcMu.RLock()
	defer funcMu.RUnlock()
	opts := make([]expr.Option, 0, len(funcRegs)+2)
	opts = append(opts, expr.AsAny(), expr.DisableAllBuiltins())
	for _, o := range funcRegs {
		opts = append(opts, o)
	}
	return opts
}

func init() {
	// 默认纯函数集（无外部依赖）；db 依赖函数（如 lookup_dict）由装配层
	// AddFunction 注入闭包。risk_score/sla_deadline 无真实数据源不注册
	//（虚标纪律——formula 引用即保存期「未知函数」拒，phase6 源到位后加函数）。
	AddFunction("now", func(params ...any) (any, error) {
		if len(params) != 0 {
			return nil, fmt.Errorf("now() 无参")
		}
		return time.Now().Format(time.RFC3339), nil
	})
	AddFunction("upper", func(params ...any) (any, error) {
		if len(params) != 1 {
			return nil, fmt.Errorf("upper(s) 须一参")
		}
		str, ok := params[0].(string)
		if !ok {
			return nil, fmt.Errorf("upper 参数须字符串")
		}
		return strings.ToUpper(str), nil
	})
}

// envIdentProbe token 级 $env 探测（B1-7 批1/N3——$env 是 expr 语言级变量非
// 内置函数，DisableAllBuiltins 不覆盖[探针实证返回全量 Fields map]；解析后拦
// 标识符本身：$env/$env.x/$env['x'] 三形态全命中 IdentifierNode.Value=="$env"，
// 字符串字面量含 "$env" 与含 env 字样的字段名零误伤）。
type envIdentProbe struct{ found bool }

func (p *envIdentProbe) Visit(node *ast.Node) {
	if id, ok := (*node).(*ast.IdentifierNode); ok && id.Value == "$env" {
		p.found = true
	}
}

// checkFormulaType 提交期结果类型校验（number=int/int64/float64、input=string；
// 其余 field_type 保存期已拒 formula，此处放行兼容）。
func checkFormulaType(f FormulaField, v any) error {
	switch f.Type {
	case "number":
		switch v.(type) {
		case float64, int, int64:
			return nil
		}
		return fmt.Errorf("formula 字段 %q（number）结果须为数字，got %T——类型漂移拒绝提交", f.Key, v)
	case "input":
		if _, ok := v.(string); ok {
			return nil
		}
		return fmt.Errorf("formula 字段 %q（input）结果须为字符串，got %T——类型漂移拒绝提交", f.Key, v)
	}
	return nil
}

// identCollector 标识符收集（保存期引用存在性/禁链式校验用——内置已全禁，
// 值位标识符都应是字段引用）。
type identCollector struct{ idents []string }

func (p *identCollector) Visit(node *ast.Node) {
	if id, ok := (*node).(*ast.IdentifierNode); ok {
		p.idents = append(p.idents, id.Value)
	}
}

// calleeCollector 调用位标识符收集（B1-6 批2——函数名不属字段引用：
// lookup_dict(...) 的 callee 是函数名非引用）。
type calleeCollector struct{ names map[string]bool }

func (c *calleeCollector) Visit(node *ast.Node) {
	if call, ok := (*node).(*ast.CallNode); ok {
		if id, ok := call.Callee.(*ast.IdentifierNode); ok {
			c.names[id.Value] = true
		}
	}
}

// Identifiers 提取表达式引用的字段标识符集（B1-7 批2 保存期校验共用件；
// B1-6 批2 起排除函数调用位标识符——与字段同名的函数名值位引用属罕见冲突，
// 校验面按引用处理不另设豁免）。
func Identifiers(src string) ([]string, error) {
	tree, err := parser.Parse(src)
	if err != nil {
		return nil, err
	}
	cc := calleeCollector{names: map[string]bool{}}
	ast.Walk(&tree.Node, &cc)
	var p identCollector
	ast.Walk(&tree.Node, &p)
	out := make([]string, 0, len(p.idents))
	for _, id := range p.idents {
		if !cc.names[id] {
			out = append(out, id)
		}
	}
	return out, nil
}

// rejectEnvIdentifier 保存与提交双防线共用（A.6 拍板=token 级非子串扫描）。
func rejectEnvIdentifier(src string) error {
	tree, err := parser.Parse(src)
	if err != nil {
		return err
	}
	var probe envIdentProbe
	ast.Walk(&tree.Node, &probe)
	if probe.found {
		return fmt.Errorf("表达式不允许引用 $env 环境变量（A.3/A.6——全量字段 dump 通道）")
	}
	return nil
}

// S3a B3-1 转换级语义引擎（03 号 §3.1/附录 A.1）：
// "from>to" 转换级键承载 condition/validator/post_actions 三段——
// condition=谁可执行（Jira 同名语义：不满足则 available 不列出/执行 403）；
// validator=执行前 expr 校验（数据侧前置条件）；post_actions=B3-3 动作库消费。
// 纯函数层：meta 解析/条件判定/expr 求值上下文组装——vitest 式表测直测。
package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/exprcalc"
)

// TransitionMeta 转换级段（"from>to" 键的值——A.1 结构）。
type TransitionMeta struct {
	// Condition 权限门（Jira condition 同语义）：roles 数组=允许执行的角色集；
	// 空数组和 nil=不限（既有行为零破坏）。首批=assignee/creator/org_admin/admin
	Roles []string `json:"roles,omitempty"`
	// Validator 执行前数据校验（expr——custom_data+内置字段上下文，结果须为 true）
	Validator string `json:"validator,omitempty"`
	// PostActions 执行后动作（B3-3 消费——本层只解析透传）
	PostActions []json.RawMessage `json:"post_actions,omitempty"`
}

// transitionKey 组装 "from>to" 键。
func transitionKey(from, to string) string { return from + ">" + to }

// parseTransitionMeta 解析类型的 transition_meta（state 级键[B10-4 timeout] 与
// 转换级键[from>to，含>] 共存同列——A.1 一列两用口径）。单条提取。
func parseTransitionMeta(raw json.RawMessage, from, to string) (*TransitionMeta, error) {
	if len(raw) == 0 {
		return nil, nil // 无 meta=既有行为
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("transition_meta 解析失败: %w", err)
	}
	seg, ok := m[transitionKey(from, to)]
	if !ok {
		return nil, nil // 该转换无配置=缺省行为
	}
	var tm TransitionMeta
	if err := json.Unmarshal(seg, &tm); err != nil {
		return nil, fmt.Errorf("transition_meta[%s] 结构不合法: %w", transitionKey(from, to), err)
	}
	return &tm, nil
}

// ValidateTransitionMeta 保存期校验（UpdateTicketType 随批——键域 ⊆ transitions
// 展开集；validator 语法 CheckSyntax；roles 词表）。零 DB 纯函数。
func ValidateTransitionMeta(states []string, transitions map[string][]string, raw json.RawMessage) error {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]json.RawMessage
	if err := json.Unmarshal(raw, &m); err != nil {
		return errcode.New(errcode.ErrInvalidParams.Code, "transition_meta 须为对象")
	}
	// 展开合法转换集（含 state 级键域——timeout_minutes 走 B10-4 校验器）
	stateSet := map[string]bool{}
	for _, s := range states {
		stateSet[s] = true
	}
	validEdges := map[string]bool{} // "from>to"
	for from, tos := range transitions {
		for _, to := range tos {
			validEdges[transitionKey(from, to)] = true
		}
	}
	for key, seg := range m {
		// 两形键：state 级（B10-4 timeout）与转换级（from>to）——按 ">" 区分
		if !strings.Contains(key, ">") {
			if !stateSet[key] {
				return errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("transition_meta 键 %q 不在 states 中", key))
			}
			continue // state 级键结构校验归 validateTransitionMeta（B10-4）
		}
		if !validEdges[key] {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("transition_meta 转换级键 %q 不在 transitions 展开集中", key))
		}
		var tm TransitionMeta
		if err := json.Unmarshal(seg, &tm); err != nil {
			return errcode.New(errcode.ErrInvalidParams.Code,
				fmt.Sprintf("transition_meta[%s] 结构不合法", key))
		}
		for _, r := range tm.Roles {
			switch r {
			case "assignee", "creator", "org_admin", "admin":
			default:
				return errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("transition_meta[%s].roles 含未知角色 %q（assignee/creator/org_admin/admin）", key, r))
			}
		}
		if tm.Validator != "" {
			if err := exprcalc.CheckSyntax(tm.Validator); err != nil {
				return errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("transition_meta[%s].validator 语法错误: %v", key, err))
			}
		}
	}
	return nil
}

// transitionActor 角色判定上下文（condition 求值用——按工单与操作人现场计算）。
type transitionActor struct {
	IsAssignee bool // 当前处理人
	IsCreator  bool // 创建人
	IsOrgAdmin bool // 工单所属 org 的 admin/owner（组内委托轴）
	IsAdmin    bool // 全局 admin/superadmin（L1 层已过——此处恒 true 走通配）
}

// checkCondition condition 权限门判定（Jira 同语义：不满足 → available 不列出/
// 执行 403）。tm=nil 或 roles 空=不限。admin 恒放行（三层鉴权既有语义）。
func checkCondition(tm *TransitionMeta, actor transitionActor) bool {
	if tm == nil || len(tm.Roles) == 0 {
		return true // 缺省=既有行为
	}
	for _, r := range tm.Roles {
		switch r {
		case "assignee":
			if actor.IsAssignee {
				return true
			}
		case "creator":
			if actor.IsCreator {
				return true
			}
		case "org_admin":
			if actor.IsOrgAdmin {
				return true
			}
		case "admin":
			if actor.IsAdmin {
				return true
			}
		}
	}
	return false
}

// buildValidatorEnv 组装 validator expr 上下文（A.1 字段引用约定：内置字段=
// 标识符直引 priority/title/status 等；动态字段=map 索引 custom_data["字段名"]）。
func buildValidatorEnv(t *model.Ticket) exprcalc.EvalEnv {
	fields := map[string]any{
		"priority":  t.Priority,
		"title":     t.Title,
		"status":    t.Status,
		"type_code": t.TypeCode,
	}
	// custom_data 整树可索引（custom_data["字段名"] 形态）
	if len(t.CustomData) > 0 {
		var cd map[string]any
		if json.Unmarshal(t.CustomData, &cd) == nil {
			fields["custom_data"] = cd
		}
	}
	return exprcalc.EvalEnv{Fields: fields}
}

// checkValidator validator 数据侧前置条件（expr 结果须为 true）。tm=nil/空=跳过。
// 返回 error 含表达式片段供 400 回显。
func checkValidator(tm *TransitionMeta, t *model.Ticket) error {
	if tm == nil || tm.Validator == "" {
		return nil
	}
	v, err := exprcalc.EvalFormula(tm.Validator, buildValidatorEnv(t))
	if err != nil {
		return errcode.New(errcode.ErrInvalidParams.Code,
			fmt.Sprintf("validator 求值失败（%s）: %v", truncate(tm.Validator, 60), err))
	}
	b, ok := v.(bool)
	if !ok || !b {
		return errcode.New(errcode.ErrInvalidParams.Code,
			fmt.Sprintf("前置条件不满足（%s）", truncate(tm.Validator, 60)))
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

// resolveActor 计算操作人的 transition 角色上下文（DB 侧查询在此方法外——
// 本函数只做纯计算，DB 判定由调用方注入）。
func resolveActor(t *model.Ticket, userID int64, roles []string, isOrgAdmin bool) transitionActor {
	actor := transitionActor{IsOrgAdmin: isOrgAdmin, IsAdmin: hasRoleCode(roles, "admin") || hasRoleCode(roles, "superadmin")}
	if t.CreatedBy == userID {
		actor.IsCreator = true
	}
	if t.AssignedTo != nil && *t.AssignedTo == userID {
		actor.IsAssignee = true
	}
	return actor
}

func hasRoleCode(roles []string, code string) bool {
	for _, r := range roles {
		if r == code {
			return true
		}
	}
	return false
}

// AvailableTransitions 列出当前用户可执行的转换（GET /tickets/:id/transitions/
// available——Jira condition 语义：按当前用户过滤；服务端 AssertTransition +
// condition 为 fail-closed 兜底，前端过滤仅 UX）。
type AvailableTransition struct {
	To string `json:"to"`
	// ConditionSatisfied=false 的转换不出现（前端按钮渲染依据=只列可点的）
	_ struct{}
}

// AvailableTransitionsFor 计算（纯函数——DB 判定 orgAdmin 由调用方预取注入）。
func AvailableTransitionsFor(ctx context.Context, t *model.Ticket, transitions map[string][]string,
	metaRaw json.RawMessage, userID int64, roles []string, isOrgAdmin bool) []string {
	actor := resolveActor(t, userID, roles, isOrgAdmin)
	tos := transitions[t.Status]
	if len(tos) == 0 {
		return nil
	}
	out := make([]string, 0, len(tos))
	for _, to := range tos {
		tm, err := parseTransitionMeta(metaRaw, t.Status, to)
		if err != nil {
			continue // 坏数据跳过（保存期有校验——此处防御手工改库）
		}
		if checkCondition(tm, actor) {
			out = append(out, to)
		}
	}
	return out
}

// S3a B3-3 首批四动作（03 号 §3.3——2026-10-24 拍板清单）。
// notify=无规则兜底（通知收敛走 S6 规则面——双通道分工）/ cc=抄送 /
// reassign=转派（扫描器事务内直调同源）/ spawn_task=提交任务（taskrunner
// durable_submit——owner「下发即起任务」场景实证）。
package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// NotifyAction 通知动作（无规则兜底档——S6 message_rules 无路由时的直发）。
type NotifyAction struct {
	// Svc 经工单服务事件出口派发 ticket.action_notify（走 MessageDispatcher
	// 编排链，不直调发送）——审计 A1 连带修复：原持 HookRegistry 但装配期
	// 无法取得（eventRegistry 构造在 NewApp 晚于动作注册）致 Reg 恒 nil
	// 静默 no-op；改持 Service（装配序早于动作注册）复用 emitTicketEvent 出口。
	Svc *Service
}

func (a *NotifyAction) Name() string                { return "notify" }
func (a *NotifyAction) Delivery() DeliverySemantics { return DeliveryAtMostOnce }

func (*NotifyAction) validateParams(params map[string]any) error {
	if template, provided := params["template"]; provided && template != nil {
		if _, ok := template.(string); !ok {
			return fmt.Errorf("template 须为字符串")
		}
	}
	if recipients, provided := params["recipients"]; provided && recipients != nil {
		if !validActionIDs(recipients, false) {
			return fmt.Errorf("recipients 须为用户 ID 数组")
		}
	}
	return nil
}

// Execute params: {"template": "code", "recipients": ["id"...]}——emit
// ticket.action_notify 事件由 S6 规则面路由；template 可选（2026-10-26 通知面收口批生效——此前参数
// 写在文档里但从未被读）：显式指定时覆盖规则默认模板（规则仍是开关与兜底）；不指定走规则/种子默认。
func (a *NotifyAction) Execute(ctx context.Context, ac ActionContext) error {
	if a.Svc == nil {
		return nil
	}
	recipients := ""
	tplCode := ""
	if ac.Params != nil {
		if list, ok := ac.Params["recipients"].([]any); ok {
			parts := idsFromParam(list)
			strs := make([]string, len(parts))
			for i, id := range parts {
				strs[i] = strconv.FormatInt(id, 10)
			}
			recipients = strings.Join(strs, ",")
		}
		if t, ok := ac.Params["template"].(string); ok {
			tplCode = t
		}
	}
	a.Svc.EmitActionNotify(ctx, ac.Ticket, ac.ActorID, recipients, tplCode)
	return nil
}

func actionInteger(value any) (int64, bool) {
	switch n := value.(type) {
	case json.Number:
		v, err := n.Int64()
		return v, err == nil
	case float64: // direct action callers may pass their own parameter map
		v := int64(n)
		return v, float64(v) == n
	default:
		return 0, false
	}
}

func validActionIDs(value any, requireOne bool) bool {
	list, ok := value.([]any)
	return ok && (!requireOne || len(list) != 0) && len(idsFromParam(list)) == len(list)
}

func actionUserID(value any) (int64, bool) {
	if raw, ok := value.(string); ok {
		id, err := strconv.ParseInt(raw, 10, 64)
		return id, err == nil && id > 0
	}
	id, ok := actionInteger(value)
	return id, ok && id > 0
}

// idsFromParam preserves integral JSON numbers and numeric string IDs.
func idsFromParam(list []any) []int64 {
	ids := make([]int64, 0, len(list))
	for _, v := range list {
		if id, ok := actionUserID(v); ok {
			ids = append(ids, id)
		}
	}
	return ids
}

// CCAction 抄送动作（复用 cc_service AddCC——读可见特例支自动生效）。
type CCAction struct {
	Svc *Service
}

// NewCCAction 构造（app 装配用——A1 修复随批）。
func NewCCAction(svc *Service) *CCAction { return &CCAction{Svc: svc} }

func (a *CCAction) Name() string                { return "cc" }
func (a *CCAction) Delivery() DeliverySemantics { return DeliveryAtMostOnce }

func (*CCAction) validateParams(params map[string]any) error {
	if !validActionIDs(params["user_ids"], true) {
		return fmt.Errorf("user_ids 须为非空用户 ID 数组")
	}
	return nil
}

// Execute params: {"user_ids": [int...]}——以动作上下文操作人身份挂抄送。
func (a *CCAction) Execute(ctx context.Context, ac ActionContext) error {
	if a.Svc == nil || ac.Params == nil {
		return nil
	}
	list, ok := ac.Params["user_ids"].([]any)
	if !ok || len(list) == 0 {
		return nil
	}
	ids := idsFromParam(list)
	if len(ids) == 0 {
		return nil
	}
	actor := ac.ActorID
	if ac.SystemAsCreator {
		actor = ac.Ticket.CreatedBy // 系统身份重放——creator 恒有「抄送他人」update 面
	}
	_, err := a.Svc.addCC(ctx, &model.AddTicketCCRequest{
		TicketID: ac.Ticket.ID, UserIDs: ids,
	}, actor, !ac.SystemAsCreator)
	return err
}

// ReassignAction 转派动作（升级链——强一致写操作；A.3 裁定：扫描器事务内直调
// 同源。此实现=转换后形态，事务=调用方转换事务之后的独立事务）。
type ReassignAction struct {
	Svc *Service
}

// NewReassignAction 构造（app 装配用——A1 修复随批）。
func NewReassignAction(svc *Service) *ReassignAction { return &ReassignAction{Svc: svc} }

func (a *ReassignAction) Name() string                { return "reassign" }
func (a *ReassignAction) Delivery() DeliverySemantics { return DeliveryAtMostOnce } // 写操作但单条 Assign 幂等可重试

func (*ReassignAction) validateParams(params map[string]any) error {
	if _, ok := actionUserID(params["user_id"]); !ok {
		return fmt.Errorf("user_id 须为正整数")
	}
	return nil
}

// Execute params: {"user_id": int}——转派到目标处理人。
func (a *ReassignAction) Execute(ctx context.Context, ac ActionContext) error {
	if a.Svc == nil || ac.Params == nil {
		return nil
	}
	uid, ok := actionUserID(ac.Params["user_id"])
	if !ok {
		return fmt.Errorf("reassign: user_id 须为正整数")
	}
	if ac.SystemAsCreator {
		// P1-2 修复（2026-11-01 审计）：canOperate("assign") 不含 isCreator——以 creator
		// 身份走 Assign 在非 admin 创建者场景必 403（TestB33 曾用 admin creator 掩盖）。
		// 改走 assignAsSystem 系统通道（G-32 委托放大族：配置型动作=获得对应写操作的
		// 委托放大，配置面已受 ticket:type:manage 控制；auditActor=真实操作人归因）。
		return a.Svc.assignAsSystem(ctx, &model.AssignTicketRequest{ID: ac.Ticket.ID, AssignedTo: &uid}, ac.ActorID)
	}
	return a.Svc.Assign(ctx, &model.AssignTicketRequest{ID: ac.Ticket.ID, AssignedTo: &uid}, ac.ActorID)
}

// SpawnTaskAction 提交任务动作（owner 2026-10-24 场景实证——「下发即起任务」；
// durable_submit：taskrunner 持久化凭证+回调幂等栅栏消费）。
type SpawnTaskAction struct {
	Client TaskSubmitter // taskrunner 提交通道接口（防 service 互 import——端口注入）
}

// NewSpawnTaskAction 构造（app 装配用——A1 修复随批）。
func NewSpawnTaskAction(client TaskSubmitter) *SpawnTaskAction {
	return &SpawnTaskAction{Client: client}
}

// TaskSubmitInput 提交请求（taskrunner_service.TaskSubmitInput 投影——域隔离
// 禁 service 互 import，端口接口用本包类型）。
type TaskSubmitInput struct {
	Action      string
	Params      json.RawMessage
	TimeoutSecs int
}

// TaskSubmitter taskrunner 提交端口（taskrunner_service.Submit 同签名投影；
// adapter 在 app 装配层做类型转换——zip 同构字段）。
type TaskSubmitter interface {
	Submit(ctx context.Context, in *TaskSubmitInput, actor, sourceIP, selfBaseURL string) (any, error)
}

func (a *SpawnTaskAction) Name() string                { return "spawn_task" }
func (a *SpawnTaskAction) Delivery() DeliverySemantics { return DeliveryDurableSubmit }

func (*SpawnTaskAction) validateParams(params map[string]any) error {
	if action, ok := params["action"].(string); !ok || strings.TrimSpace(action) == "" {
		return fmt.Errorf("action 须为非空字符串")
	}
	if values, provided := params["params"]; provided && values != nil {
		if _, ok := values.(map[string]any); !ok {
			return fmt.Errorf("params 须为对象")
		}
	}
	if value, provided := params["timeout_secs"]; provided && value != nil {
		if seconds, ok := actionInteger(value); !ok || seconds < 0 || seconds > 86400 {
			return fmt.Errorf("timeout_secs 须为 0 到 86400 的整数")
		}
	}
	return nil
}

// Execute params: {"action": "action_id", "params": {...}, "timeout_secs": 30}——
// 提交一次性任务；params 支持 {"from": "ticket.field"} 简单取数（event/result
// 全量映射随 B3-3 后批）。
func (a *SpawnTaskAction) Execute(ctx context.Context, ac ActionContext) error {
	if a.Client == nil || ac.Params == nil {
		return fmt.Errorf("spawn_task: 未装配 taskrunner 通道")
	}
	actionID, _ := ac.Params["action"].(string)
	if actionID == "" {
		return fmt.Errorf("spawn_task: action 缺失")
	}
	var taskParams json.RawMessage
	if tp, ok := ac.Params["params"].(map[string]any); ok && len(tp) > 0 {
		b, err := json.Marshal(tp)
		if err != nil {
			return fmt.Errorf("spawn_task: params 序列化失败: %w", err)
		}
		taskParams = b
	}
	timeout := 0
	if value, provided := ac.Params["timeout_secs"]; provided && value != nil {
		tv, ok := actionInteger(value)
		if !ok || tv < 0 || tv > 86400 {
			return fmt.Errorf("spawn_task: timeout_secs 须为 0 到 86400 的整数")
		}
		timeout = int(tv)
	}
	_, err := a.Client.Submit(ctx, &TaskSubmitInput{
		Action: actionID, Params: taskParams, TimeoutSecs: timeout,
	}, fmt.Sprintf("action:%d", ac.ActorID), "", "")
	return err
}

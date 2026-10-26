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
	// 静默 no-op；改持 Service（装配序早于动作注册）复用 emitEvent 出口。
	Svc *Service
}

func (a *NotifyAction) Name() string                { return "notify" }
func (a *NotifyAction) Delivery() DeliverySemantics { return DeliveryAtMostOnce }

// Execute params: {"template": "code", "recipients": ["id"...]}——emit
// ticket.action_notify 事件由 S6 规则面路由（无规则则该动作静默——设计内）。
func (a *NotifyAction) Execute(ctx context.Context, ac ActionContext) error {
	if a.Svc == nil {
		return nil
	}
	recipients := ""
	if ac.Params != nil {
		if list, ok := ac.Params["recipients"].([]any); ok {
			// JSON 数字解出为 float64——必须经 int64 再格式化：fmt.Sprint(float64(1000000))="1e+06"，
			// 用户 ID ≥1e6 时收件人串全部损坏（CCAction 同款转换，此处曾漏）
			parts := idsFromParam(list)
			strs := make([]string, len(parts))
			for i, id := range parts {
				strs[i] = strconv.FormatInt(id, 10)
			}
			recipients = strings.Join(strs, ",")
		}
	}
	a.Svc.EmitActionNotify(ctx, ac.Ticket.ID, ac.ActorID, ac.Ticket.Title,
		ac.Ticket.HumanKey, recipients)
	return nil
}

// idsFromParam 动作参数里的 ID 列表（JSON 解出的 float64 / 数字字符串）→ int64；非法项跳过。
func idsFromParam(list []any) []int64 {
	ids := make([]int64, 0, len(list))
	for _, v := range list {
		switch x := v.(type) {
		case float64:
			ids = append(ids, int64(x))
		case string:
			if n, err := strconv.ParseInt(x, 10, 64); err == nil {
				ids = append(ids, n)
			}
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
	_, err := a.Svc.AddCC(ctx, &model.AddTicketCCRequest{
		TicketID: ac.Ticket.ID, UserIDs: ids,
	}, actor)
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

// Execute params: {"user_id": int}——转派到目标处理人。
func (a *ReassignAction) Execute(ctx context.Context, ac ActionContext) error {
	if a.Svc == nil || ac.Params == nil {
		return nil
	}
	f, ok := ac.Params["user_id"].(float64)
	if !ok {
		return fmt.Errorf("reassign: user_id 缺失")
	}
	uid := int64(f)
	actor := ac.ActorID
	if ac.SystemAsCreator {
		actor = ac.Ticket.CreatedBy
	}
	return a.Svc.Assign(ctx, &model.AssignTicketRequest{ID: ac.Ticket.ID, AssignedTo: &uid}, actor)
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
	if tv, ok := ac.Params["timeout_secs"].(float64); ok {
		timeout = int(tv)
	}
	_, err := a.Client.Submit(ctx, &TaskSubmitInput{
		Action: actionID, Params: taskParams, TimeoutSecs: timeout,
	}, fmt.Sprintf("action:%d", ac.ActorID), "", "")
	return err
}

package msgrender

// Variable is the single server-owned dictionary used by save validation,
// API documentation and the renderer's required-builtin classification.
type Variable struct {
	Path        string
	Description string
}

var variables = []Variable{
	{Path: "ticket.title", Description: "工单标题"},
	{Path: "ticket.type_code", Description: "工单类型编码"},
	{Path: "ticket.priority", Description: "优先级（1-4）"},
	{Path: "ticket.status", Description: "当前状态"},
	{Path: "ticket.human_key", Description: "人读编号（TK-yyyymmdd-NNNN）"},
	{Path: "ticket.link", Description: "工单链接"},
	{Path: "ticket.from_status", Description: "转换前状态（transitioned）"},
	{Path: "ticket.to_status", Description: "转换后状态（transitioned）"},
	{Path: "ticket.from_assignee", Description: "分派前处理人 ID"},
	{Path: "ticket.to_assignee", Description: "分派后处理人 ID"},
	{Path: "ticket.cc_users", Description: "抄送人 ID 列表（逗号分隔）"},
	{Path: "ticket.reminder_count", Description: "催办次数（timeout）"},
	{Path: "ticket.overdue_minutes", Description: "已超时分钟数（timeout）"},
	{Path: "ticket.actor", Description: "操作人 ID"},
	{Path: "ticket.creator", Description: "工单创建人 ID（与操作人区分）"},
	{Path: "event.job", Description: "定时任务名（ops.cron_failed）"},
	{Path: "event.error", Description: "错误信息（ops.cron_failed / ops.action_failed）"},
	{Path: "event.last_success", Description: "上次成功指引（ops.cron_failed）"},
	{Path: "event.action", Description: "失败动作名（ops.action_failed）"},
	{Path: "event.ticket_id", Description: "关联工单 ID（ops.action_failed）"},
	{Path: "event.actor", Description: "触发人 ID（ops.action_failed）"},
	{Path: "event.level", Description: "容量档位（ops.storage_usage_high）"},
	{Path: "event.ratio", Description: "用量占比（ops.storage_usage_high）"},
	{Path: "event.usage_bytes", Description: "已用字节（ops.storage_usage_high）"},
	{Path: "event.capacity_bytes", Description: "容量字节（ops.storage_usage_high）"},
}

func BuiltinVariables() []Variable { return append([]Variable(nil), variables...) }
func BuiltinPaths() map[string]bool {
	out := make(map[string]bool, len(variables))
	for _, v := range variables {
		out[v.Path] = true
	}
	return out
}

package ticket

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

// 审计 #8：标题渲染须单次扫描——替换入的值不得再被当作占位展开（修前按 payload 逐键 ReplaceAll，
// 工单标题含 "{{human_key}}" 字面即可能被二次展开，且 map 遍历随机序致结果不定）。
func TestRenderTitle_SinglePassNoReexpansion(t *testing.T) {
	ev := domainevent.DomainEvent{Payload: domainevent.Payload{
		"title": "含占位字面 {{human_key}} 的标题", "human_key": "TK-1",
	}}
	assert.Equal(t, "工单已更新：含占位字面 {{human_key}} 的标题", renderTitle("工单已更新：{{title}}", ev))
	assert.Equal(t, "缺键 {{nope}} 原样", renderTitle("缺键 {{nope}} 原样", ev))
	assert.Equal(t, "TK-1 / TK-1", renderTitle("{{human_key}} / {{human_key}}", ev))
}

// 审计 #8：处理人编辑/评论自己的单不给自己落「工单已更新」；自派/抄送我仍落（TestB67 既定语义）。
func TestCollectRecipients_SelfNoiseActionsSkipActor(t *testing.T) {
	ev := func(a domainevent.Action) domainevent.DomainEvent {
		return domainevent.DomainEvent{Action: a, ActorID: 7,
			Payload: domainevent.Payload{"to_assignee": "7", "cc_users": "7,8,8"}}
	}
	assert.Equal(t, []int64{8}, collectRecipients(ev(domainevent.TicketUpdated)), "自编辑不自收")
	assert.Equal(t, []int64{8}, collectRecipients(ev(domainevent.TicketCommented)), "自评论不自收")
	assert.Equal(t, []int64{7, 8}, collectRecipients(ev(domainevent.TicketAssigned)), "自派保留且去重")
	assert.Equal(t, []int64{7, 8}, collectRecipients(ev(domainevent.TicketCCAdded)), "抄送我保留")
}

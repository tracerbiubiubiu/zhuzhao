//go:build integration

// S10 B10-1 抄送集成测试：L2 特例通道四向 + 幂等 + 通知链（10 号 §8 场景标尺/A.7）。
// 特例支语义分层锚点：cc 只进读可见集（列表/详情），不进操作授权集（update→403）。
package ticket

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestB101_TicketCC(t *testing.T) {
	ctx := context.Background()

	// 通知链组装（b65 同款）：注册表 + 种子规则路由（000062）+ 捕获 sender
	reg := domainevent.NewHookRegistry(nil)
	sender := &captureSender{}
	hook := NewTicketNotifyHook(
		repository.NewMessageTemplateRepo(testPool), repository.NewMessageRuleRepo(testPool),
		repository.NewMessageLogRepo(testPool), repository.NewNotificationOptoutRepo(testPool),
		sender, nil, nil)
	reg.Subscribe(domainevent.BizTicket, "b101.notify", hook)

	svc, aid, bid, vid, _ := setupTicket2a(t)
	svc.SetEventRegistry(reg)
	oid := childOrgID(t, "b101")
	tk := newTicketHelper(t, svc, aid, oid, "B10-1 抄送四向")

	// sees：uid 的列表是否含 ticketID（GetFilter 特例支断言——走列表真实链路）
	sees := func(uid, ticketID int64) bool {
		l, err := svc.List(ctx, model.TicketListQuery{Page: 1, PageSize: 200}, uid)
		require.NoError(t, err)
		for _, row := range l.List {
			if row.ID == ticketID {
				return true
			}
		}
		return false
	}

	t.Run("基线：org 外非抄送用户不可见（404 同形）", func(t *testing.T) {
		assert.False(t, sees(vid, tk.ID), "抄送前 V 不应看到该单")
		_, err := svc.Get(ctx, tk.ID, vid)
		requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	})

	t.Run("不可见用户自抄送 → 404（特例支不旁路可见性前置）", func(t *testing.T) {
		_, err := svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{vid}}, vid)
		requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	})

	t.Run("属主抄他人 → 列表可见 + 详情可开 + 通知到人（特例支正向）", func(t *testing.T) {
		rows, err := svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{vid}}, aid)
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.EqualValues(t, vid, rows[0].UserID)

		assert.True(t, sees(vid, tk.ID), "抄送后 V 应经特例支看到该单")
		got, err := svc.Get(ctx, tk.ID, vid)
		require.NoError(t, err)
		assert.EqualValues(t, tk.ID, got.ID)

		// 通知链：cc_added → 种子规则（000062）→ 渲染 → 发送（收件人=本次新增抄送人）
		time.Sleep(200 * time.Millisecond) // 异步派发
		ccMsgs := func() []string {
			var recipients []string
			for _, m := range sender.messages() {
				if m.TemplateCode == "ticket_cc_notify" {
					recipients = append(recipients, m.Recipients...)
				}
			}
			return recipients
		}()
		assert.Contains(t, ccMsgs, strconv.FormatInt(vid, 10), "cc 通知应收给被抄送人")
	})

	t.Run("抄送人改单/抄他人/删单 → 403（能看不能改）", func(t *testing.T) {
		newTitle := "越权改单"
		_, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: &newTitle}, vid)
		requireErrCode(t, err, errcode.ErrNoPermission.Code)

		_, err = svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{bid}}, vid)
		requireErrCode(t, err, errcode.ErrNoPermission.Code)

		err = svc.Delete(ctx, tk.ID, vid)
		requireErrCode(t, err, errcode.ErrNoPermission.Code)
	})

	t.Run("摘除后可见性即时回收", func(t *testing.T) {
		require.NoError(t, svc.RemoveCC(ctx, &model.RemoveTicketCCRequest{TicketID: tk.ID, UserID: vid}, aid))
		assert.False(t, sees(vid, tk.ID), "摘除后 V 不应再看到该单")
		_, err := svc.Get(ctx, tk.ID, vid)
		requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
	})

	t.Run("幂等：重复挂载单行不重发；自摘幂等；幻影用户 400", func(t *testing.T) {
		_, err := svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{vid, vid}}, aid)
		require.NoError(t, err)
		rows, err := svc.ListCC(ctx, tk.ID, aid)
		require.NoError(t, err)
		count := 0
		for _, r := range rows {
			if r.UserID == vid {
				count++
			}
		}
		assert.Equal(t, 1, count, "同一用户重复挂载应幂等为单行")

		// 幻影用户 FK 23503 → 400
		_, err = svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{999999999}}, aid)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)

		// 本人自摘（读可见面——cc 支放行）；摘后再自摘 → 404 同形（已不可见）；
		// 有权者（属主）再摘 → 幂等无错
		require.NoError(t, svc.RemoveCC(ctx, &model.RemoveTicketCCRequest{TicketID: tk.ID, UserID: vid}, vid))
		err = svc.RemoveCC(ctx, &model.RemoveTicketCCRequest{TicketID: tk.ID, UserID: vid}, vid)
		requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
		require.NoError(t, svc.RemoveCC(ctx, &model.RemoveTicketCCRequest{TicketID: tk.ID, UserID: vid}, aid))
		assert.False(t, sees(vid, tk.ID))
	})
}

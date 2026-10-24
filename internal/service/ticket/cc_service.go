// S10 B10-1 工单抄送（10 号 §3.1 + 附录 A.2）：加/删/列表 + ticket.cc_added 事件。
//
// 授权口径（A.2 语义分层的 service 面落点）：
//   - 自抄送（user_ids 仅本人）/摘除本人/看抄送列表 = **读可见**（Authorize read——
//     cc 特例支使被抄送人可自看自摘）；
//   - 抄他人/摘他人 = **update 权**（能看不能改的反面收口：扩散可见性是写面动作）。
//
// 通知零新代码：落库后 emit ticket.cc_added（DomainEvent）→ S6 message_rules
// 种子路由（ticket_cc_notify / recipient_type=cc）走渲染→发送→message_log 既有链。
package ticket

import (
	"context"
	"strconv"
	"strings"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"time"
)

// AddCC 挂抄送（幂等——重复挂载不重发通知：事件只对本次新增的人生效）。
// 返回挂载后的全量抄送行（前端即用）。
func (s *Service) AddCC(ctx context.Context, req *model.AddTicketCCRequest, actorUserID int64) ([]model.TicketCCRow, error) {
	onlySelf := len(req.UserIDs) == 1 && req.UserIDs[0] == actorUserID
	action := "read"
	if !onlySelf {
		action = "update"
	}
	if err := s.authorizeCheck(ctx, actorUserID, action, strconv.FormatInt(req.TicketID, 10)); err != nil {
		return nil, err
	}
	added, err := s.ccRepo.Add(ctx, req.TicketID, actorUserID, req.UserIDs)
	if err != nil {
		return nil, err
	}
	if len(added) > 0 {
		s.emitCCAdded(ctx, req.TicketID, actorUserID, added)
	}
	return s.ccRepo.ListByTicket(ctx, req.TicketID)
}

// RemoveCC 摘抄送（幂等——不存在不算错）。
func (s *Service) RemoveCC(ctx context.Context, req *model.RemoveTicketCCRequest, actorUserID int64) error {
	action := "read"
	if req.UserID != actorUserID {
		action = "update"
	}
	if err := s.authorizeCheck(ctx, actorUserID, action, strconv.FormatInt(req.TicketID, 10)); err != nil {
		return err
	}
	_, err := s.ccRepo.Remove(ctx, req.TicketID, req.UserID)
	return err
}

// ListCC 抄送列表（读可见面——非可见用户 404 同形）。
func (s *Service) ListCC(ctx context.Context, ticketID, actorUserID int64) ([]model.TicketCCRow, error) {
	if err := s.authorizeCheck(ctx, actorUserID, "read", strconv.FormatInt(ticketID, 10)); err != nil {
		return nil, err
	}
	return s.ccRepo.ListByTicket(ctx, ticketID)
}

// emitCCAdded 抄送事件（事务性弱于状态流转——单条 INSERT 自带原子性，提交后派发；
// ticket 缺失=外键已删单竞态，事件面 best-effort 跳过）。
func (s *Service) emitCCAdded(ctx context.Context, ticketID, actorUserID int64, added []int64) {
	ticket, err := s.ticketRepo.GetByID(ctx, ticketID)
	if err != nil {
		return
	}
	ids := make([]string, len(added))
	for i, u := range added {
		ids[i] = strconv.FormatInt(u, 10)
	}
	payload := domainevent.Payload{
		"title": ticket.Title, "type_code": ticket.TypeCode, "status": ticket.Status,
	}
	if ticket.HumanKey != "" {
		payload["human_key"] = ticket.HumanKey
	}
	// recipient_type=cc 收件人档锚点（resolveRecipientsFromRule case "cc"）
	payload["cc_users"] = strings.Join(ids, ",")
	payload["actor"] = strconv.FormatInt(actorUserID, 10)
	s.emitEvent(domainevent.DomainEvent{
		BizType: domainevent.BizTicket, Action: domainevent.TicketCCAdded,
		BizID: strconv.FormatInt(ticketID, 10), ActorID: actorUserID,
		Payload:   payload,
		Timestamp: time.Now(),
	})
}

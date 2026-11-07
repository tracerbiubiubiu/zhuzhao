//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestNotificationCreator_ServiceEvents(t *testing.T) {
	for _, kind := range []string{"created", "updated", "assigned", "closed", "transitioned", "cc_added", "action_notify", "dispatch_missed", "timeout", "deadline"} {
		t.Run(kind, func(t *testing.T) {
			ctx := context.Background()
			svc, creator, actor, assignee, roles := setupTicket2a(t)
			roles[creator], roles[actor] = []string{"admin"}, []string{"admin"}
			oid := childOrgID(t, "creator")
			code := "creator_" + uniqueSuffix()
			_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			t.Cleanup(func() {
				_, err := testPool.Exec(context.Background(), `UPDATE ticket_types SET is_active=false WHERE code=$1`, code)
				require.NoError(t, err)
			})
			reg := domainevent.NewHookRegistry(nil)
			t.Cleanup(func() {
				ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
				defer cancel()
				require.NoError(t, reg.Drain(ctx))
			})
			events := make(chan domainevent.DomainEvent, 4)
			action := domainevent.Action("ticket." + kind)
			reg.Subscribe(domainevent.BizTicket, "creator.capture", domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) {
				if ev.Action == action {
					events <- ev
				}
			}))
			svc.SetEventRegistry(reg)
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "creator snapshot", Priority: 2, OrgID: oid}, creator)
			require.NoError(t, err)
			switch kind {
			case "created":
				actor = creator // Creation itself establishes the immutable creator.
			case "updated":
				title := "changed by another person"
				_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: &title}, actor)
			case "assigned":
				err = svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, actor)
			case "closed":
				err = svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, actor)
			case "transitioned":
				_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "closed"}, actor)
			case "cc_added":
				_, err = svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: tk.ID, UserIDs: []int64{assignee}}, actor)
			case "action_notify":
				err = (&NotifyAction{Svc: svc}).Execute(ctx, ActionContext{Ticket: tk, ActorID: actor})
			case "dispatch_missed":
				NewDispatchService(repository.NewAssignmentRuleRepo(testPool), svc, nil).OnAfterCreate(ctx, tk)
				actor = 0 // No matching dispatch rule has no human action actor.
			case "timeout", "deadline":
				meta := json.RawMessage(`{"assigned":{"timeout_minutes":1}}`)
				if kind == "deadline" {
					meta = json.RawMessage(`{"assigned":{"sla_due_hours":1}}`)
				}
				_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: meta})
				require.NoError(t, err)
				require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, actor))
				backdateStatusChanged(t, tk.ID, 2*time.Hour)
				if kind == "deadline" {
					_, err = testPool.Exec(ctx, `UPDATE tickets SET sla_due_at=NOW()-interval '1 hour' WHERE id=$1`, tk.ID)
					require.NoError(t, err)
				}
				err = NewTimeoutScanner(testPool, reg, false, nil).Handle(ctx, nil)
				actor = 0
			}
			require.NoError(t, err)
			select {
			case ev := <-events:
				require.Equal(t, strconv.FormatInt(tk.ID, 10), ev.BizID)
				require.Equal(t, []string{strconv.FormatInt(creator, 10)}, resolveRecipientsFromRule(ev, "creator", ""))
				require.Equal(t, actor, ev.ActorID)
				if actor > 0 {
					require.Equal(t, strconv.FormatInt(actor, 10), ev.Payload["actor"])
				}
			case <-time.After(2 * time.Second):
				t.Fatal("real service did not publish the selected event")
			}
		})
	}
}

func TestNotificationCreator_QueuedCloseSurvivesTicketDeletion(t *testing.T) {
	ctx := context.Background()
	svc, creator, actor, _, roles := setupTicket2a(t)
	roles[creator], roles[actor] = []string{"admin"}, []string{"admin"}
	code := "creator_deleted_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	oid := childOrgID(t, "creator_deleted")
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "queued before deletion", Priority: 2, OrgID: oid}, creator)
	require.NoError(t, err)
	var prior string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT recipient_type FROM message_rules WHERE biz_type='ticket' AND event_action='ticket.closed'`).Scan(&prior))
	_, err = testPool.Exec(ctx, `UPDATE message_rules SET recipient_type='creator' WHERE biz_type='ticket' AND event_action='ticket.closed'`)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), `UPDATE message_rules SET recipient_type=$1 WHERE biz_type='ticket' AND event_action='ticket.closed'`, prior)
		require.NoError(t, err)
	})
	sender := &captureSender{}
	hook := NewTicketNotifyHook(repository.NewMessageTemplateRepo(testPool), repository.NewMessageRuleRepo(testPool), repository.NewMessageLogRepo(testPool), repository.NewNotificationOptoutRepo(testPool), sender, nil, nil)
	reg := domainevent.NewHookRegistry(nil)
	release := make(chan struct{})
	defer close(release)
	reg.Subscribe(domainevent.BizTicket, "creator.queued", domainevent.HookFunc(func(c context.Context, ev domainevent.DomainEvent) {
		if ev.Action == domainevent.TicketClosed {
			<-release
			hook.OnDomainEvent(c, ev)
		}
	}))
	svc.SetEventRegistry(reg)
	require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, actor))
	require.NoError(t, svc.Delete(ctx, tk.ID, actor))
	// Release the already queued immutable event after the real Delete has committed.
	release <- struct{}{}
	drain, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	require.NoError(t, reg.Drain(drain))
	msgs := sender.messages()
	require.Len(t, msgs, 1)
	require.Equal(t, []string{strconv.FormatInt(creator, 10)}, msgs[0].Recipients)
}

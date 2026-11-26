//go:build integration

package ticket

import (
	"context"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func riskNotificationTicket(t *testing.T, state, title string) (*model.Ticket, int64) {
	t.Helper()
	ctx := context.Background()
	svc, creator, recipient, _, roles := setupTicket2a(t)
	roles[creator] = []string{"admin"}
	org := childOrgID(t, "risk_notice")
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: "incident", Title: title, Priority: 2, OrgID: org}, creator)
	require.NoError(t, err)
	if state == "ordinary" {
		return tk, recipient
	}
	suffix := uniqueSuffix()
	_, err = testPool.Exec(ctx, `
		INSERT INTO risk_dispatch_groups
		    (finding_key, owner_user_id, management_org_id, target_org_id, type_code, definition_version,
		     approved_scope, group_key, ticket_id, ticket_ref_snapshot, operator_user_id, ended_at)
		VALUES ($1::text, $2, $3, $3, $4, $5, 'direct', md5($1::text) || md5($1::text), $6, $7, $2,
		        CASE WHEN $8 THEN now() ELSE NULL END)`,
		suffix, creator, org, tk.TypeCode, tk.DefinitionVersion, tk.ID, tk.HumanKey, state != "active")
	require.NoError(t, err)
	if state == "deleted" {
		// Dispatch records the numeric ID alongside the human key in its immutable
		// successful group receipt, including when the group later loses its FK.
		_, err = testPool.Exec(ctx, `
			WITH plan AS (
				INSERT INTO risk_dispatch_plans (id, actor_id, plan_hash, binding, created_at, expires_at)
				VALUES (gen_random_uuid(), $2, md5($1::text) || md5($1::text), '{}', now(), now() + interval '1 hour')
				RETURNING id, plan_hash
			), op AS (
				INSERT INTO risk_operations (actor_id, operation_id, request_hash, command, result_status)
				VALUES ($2, $1::text, decode(md5($1::text) || md5($1::text), 'hex'), 'dispatch', 'completed')
				RETURNING actor_id, operation_id
			), dispatch AS (
				INSERT INTO risk_dispatch_operations (actor_id, operation_id, plan_id, plan_hash)
				SELECT op.actor_id, op.operation_id, plan.id, plan.plan_hash FROM op CROSS JOIN plan
				RETURNING actor_id, operation_id
			)
			INSERT INTO risk_dispatch_group_receipts (actor_id, operation_id, group_key, result)
			SELECT actor_id, operation_id, md5($1::text) || md5($1::text),
			       jsonb_build_object('ticket_id', $3::bigint, 'ticket_ref', $4::text, 'status', 'completed')
			FROM dispatch`, suffix, creator, tk.ID, tk.HumanKey)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, tk.ID)
		require.NoError(t, err)
	}
	return tk, recipient
}

func TestRiskTicketInboxProjection(t *testing.T) {
	ctx := context.Background()
	inbox := NewInboxService(testPool, 90, nil)
	for _, state := range []string{"active", "ended", "deleted", "ordinary"} {
		t.Run(state, func(t *testing.T) {
			const marker = "synthetic protected ticket text"
			tk, recipient := riskNotificationTicket(t, state, marker)
			// Bypass the new INSERT projection to represent the pre-fix history.
			_, err := testPool.Exec(ctx, `
				INSERT INTO notifications (recipient_id, biz_type, biz_id, title, link)
				VALUES ($1, 'ticket', $2, $3, $4)`,
				recipient, tk.ID, marker, "/legacy/"+marker)
			require.NoError(t, err)
			id := strconv.FormatInt(tk.ID, 10)
			inbox.OnDomainEvent(ctx, domainevent.DomainEvent{
				BizType: domainevent.BizTicket, Action: domainevent.TicketAssigned, BizID: id,
				Payload: domainevent.Payload{"title": marker, "to_assignee": strconv.FormatInt(recipient, 10)},
			})
			var stored string
			require.NoError(t, testPool.QueryRow(ctx, `
				SELECT title FROM notifications WHERE recipient_id=$1 AND biz_id=$2 ORDER BY id DESC LIMIT 1`,
				recipient, tk.ID).Scan(&stored))
			rows, total, err := inbox.Inbox(ctx, recipient, false, 1, 20)
			require.NoError(t, err)
			require.EqualValues(t, 2, total)
			require.Len(t, rows, 2)
			if state == "ordinary" {
				assert.Contains(t, stored, marker)
				assert.Equal(t, marker, rows[1].Title)
				assert.Equal(t, "/legacy/"+marker, rows[1].Link)
				return
			}
			assert.NotContains(t, stored, marker, "new inbox rows must be neutral in storage")
			for _, row := range rows {
				assert.NotEmpty(t, row.Title)
				assert.NotContains(t, row.Title, marker)
				assert.Equal(t, "/tickets/"+id, row.Link)
			}
		})
	}
}

func TestRiskTicketMessageProjection(t *testing.T) {
	ctx := context.Background()
	ledger := repository.NewMessageLogRepo(testPool)
	for _, state := range []string{"active", "ended", "deleted", "ordinary"} {
		t.Run(state, func(t *testing.T) {
			const marker = "synthetic protected ticket text"
			tk, recipient := riskNotificationTicket(t, state, marker)
			id := strconv.FormatInt(tk.ID, 10)
			tpl := "risk_notice_" + uniqueSuffix()
			_, err := testPool.Exec(ctx, `
				INSERT INTO message_templates (code, name, channel, subject, body, enabled)
				VALUES ($1, 'fixture', 'uip', '{{ticket.title}}', '{{ticket.description}} {{ticket.link}}', true)`, tpl)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `
				INSERT INTO message_log (template_code, biz_type, biz_id, channel, recipients, subject, body, status, error)
				VALUES ($1, 'ticket', $2, 'uip', $3, $4, $4, 'failed', $4)`,
				tpl, id, strconv.FormatInt(recipient, 10), marker)
			require.NoError(t, err)
			sender := &captureSender{}
			dispatcher := NewMessageDispatcher(repository.NewMessageTemplateRepo(testPool),
				repository.NewMessageRuleRepo(testPool), ledger, nil, sender, nil, nil)
			dispatcher.Dispatch(ctx, domainevent.DomainEvent{
				BizType: domainevent.BizTicket, Action: domainevent.TicketAssigned, BizID: id,
				Payload: domainevent.Payload{
					"title": marker, "description": marker, "link": "/unsafe/" + marker,
					"template_code": tpl, "to_assignee": strconv.FormatInt(recipient, 10),
				},
			})
			messages := sender.messages()
			require.Len(t, messages, 1)
			entries, total, err := ledger.List(ctx, 1, 10, "ticket", tpl, "", id)
			require.NoError(t, err)
			require.EqualValues(t, 2, total)
			require.Len(t, entries, 2)
			if state == "ordinary" {
				assert.Contains(t, messages[0].Subject, marker)
				assert.Contains(t, messages[0].Body, marker)
				assert.Equal(t, marker, entries[1].Error)
				return
			}
			assert.Equal(t, []string{strconv.FormatInt(recipient, 10)}, messages[0].Recipients)
			assert.Equal(t, tpl, messages[0].TemplateCode)
			assert.NotContains(t, messages[0].Subject, marker)
			assert.NotContains(t, messages[0].Body, marker)
			assert.Contains(t, messages[0].Body, "/tickets/"+id)
			for _, entry := range entries {
				assert.NotContains(t, entry.Subject, marker)
				assert.NotContains(t, entry.Body, marker)
				assert.NotContains(t, entry.Error, marker)
			}
		})
	}
}

func TestRiskTicketWorkflowFailureNotification(t *testing.T) {
	ctx := context.Background()
	const marker = "synthetic protected workflow error"
	tk, recipient := riskNotificationTicket(t, "active", "old ticket title")
	id := strconv.FormatInt(tk.ID, 10)
	tpl := "risk_action_" + uniqueSuffix()
	_, err := testPool.Exec(ctx, `
		INSERT INTO message_templates (code, name, biz_type, channel, subject, body, enabled)
		VALUES ($1, 'fixture', 'ops', 'uip', 'failure', '{{event.error}}', true)`, tpl)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `
		INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type, enabled)
		VALUES ('ops', 'ops.action_failed', $1, 'creator', true)`, tpl)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(ctx, `DELETE FROM message_rules WHERE template_code=$1`, tpl)
		require.NoError(t, err)
	})
	ledger := repository.NewMessageLogRepo(testPool)
	sender := &captureSender{}
	dispatcher := NewMessageDispatcher(repository.NewMessageTemplateRepo(testPool),
		repository.NewMessageRuleRepo(testPool), ledger, nil, sender, nil, nil)
	dispatcher.Dispatch(ctx, domainevent.DomainEvent{
		BizType: domainevent.BizOps, Action: domainevent.ActionOpsActionFailed, BizID: id,
		Payload: domainevent.Payload{"error": marker, "actor": strconv.FormatInt(recipient, 10)},
	})
	messages := sender.messages()
	require.Len(t, messages, 1)
	assert.NotContains(t, messages[0].Body, marker)
	assert.Contains(t, messages[0].Body, "/tickets/"+id)
}

//go:build integration

package ticket

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/humankey"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestRiskBridgeCommitsWithOriginalDispatcherAndRollsBackOnLinkError(t *testing.T) {
	svc, dispatcher, org := setupTicketAdmin(t)
	svc.keyGen = humankey.New(testPool, "B2")
	ctx := context.Background()
	targetOrg := org()
	code := "b2_bridge_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "Synthetic remediation"})
	require.NoError(t, err)
	definition := publishD01(t, svc, code, dispatcher)
	var owner int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO users (username, password, status) VALUES ($1, 'test-hash', 1)
		RETURNING id
	`, code).Scan(&owner))

	create := func(title string, afterLink error) (*model.Ticket, error) {
		request := &model.CreateTicketRequest{
			TypeCode: code, Title: title, Description: "synthetic dispatch body", OrgID: targetOrg, Priority: 2,
			DefinitionVersion: strconv.FormatInt(definition.DefinitionVersion, 10), DefinitionHash: definition.DefinitionHash,
		}
		prepared, err := svc.PrepareRiskDispatchCreate(ctx, request, dispatcher)
		if err != nil {
			return nil, err
		}
		var created *model.Ticket
		err = repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
			locked, err := svc.LockRiskDispatchCreateTx(ctx, tx, prepared)
			if err != nil {
				return err
			}
			created, err = svc.CreateRiskDispatchTicketTx(ctx, tx, locked, owner, func(ctx context.Context, tx pgx.Tx, ticketID int64) error {
				var reference string
				if err := tx.QueryRow(ctx, "SELECT human_key FROM tickets WHERE id = $1", ticketID).Scan(&reference); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `
					INSERT INTO risk_dispatch_groups
						(finding_key, owner_user_id, management_org_id, target_org_id, type_code, definition_version,
						 approved_scope, group_key, ticket_id, ticket_ref_snapshot, operator_user_id)
					VALUES ($1, $2, $3, $3, $4, $5, 'test', $6, $7, $9, $8)
				`, title, owner, targetOrg, code, definition.DefinitionVersion, strings.Repeat("a", 64), ticketID, dispatcher, reference)
				if err != nil {
					return err
				}
				return afterLink
			})
			return err
		})
		return created, err
	}

	linked, err := create("risk bridge successful", nil)
	require.NoError(t, err)
	var createdBy, eventActor, groupOwner, assignee, assignedBy int64
	var snapshotRef string
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT t.created_by, e.user_id, g.owner_user_id, t.assigned_to, a.user_id, g.ticket_ref_snapshot
		FROM tickets t
		JOIN ticket_events e ON e.ticket_id = t.id AND e.action = 'created'
		JOIN risk_dispatch_groups g ON g.ticket_id = t.id
		JOIN ticket_events a ON a.ticket_id = t.id AND a.action = 'assigned'
		WHERE t.id = $1
	`, linked.ID).Scan(&createdBy, &eventActor, &groupOwner, &assignee, &assignedBy, &snapshotRef))
	require.Equal(t, owner, createdBy)
	require.Equal(t, dispatcher, eventActor)
	require.Equal(t, owner, groupOwner)
	require.Equal(t, owner, assignee)
	require.Equal(t, dispatcher, assignedBy)
	require.NotEmpty(t, snapshotRef)
	require.Equal(t, linked.HumanKey, snapshotRef)

	t.Run("controlled_port_persists_neutral_summary", func(t *testing.T) {
		var title, description string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT title, description FROM tickets WHERE id=$1`, linked.ID).Scan(&title, &description))
		require.Equal(t, "漏洞整改", title)
		require.Equal(t, "请在风险模块查看有权访问的整改事项。", description)
		require.Equal(t, title, linked.Title)
		require.Equal(t, description, linked.Description)
	})

	linkErr := errors.New("synthetic link operation error")
	_, err = create("risk bridge rollback", linkErr)
	require.ErrorIs(t, err, linkErr)
	var tickets, groups int
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT (SELECT count(*) FROM tickets WHERE type_code = $1),
		       (SELECT count(*) FROM risk_dispatch_groups WHERE type_code = $1)
	`, code).Scan(&tickets, &groups))
	require.Equal(t, 1, tickets)
	require.Equal(t, 1, groups)

	const protectedTitle = "synthetic historical title sentinel"
	const protectedDescription = "synthetic historical description sentinel"
	_, err = testPool.Exec(ctx, `UPDATE risk_dispatch_groups SET ended_at=now() WHERE ticket_id=$1`, linked.ID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE tickets SET title=$2, description=$3 WHERE id=$1`, linked.ID, protectedTitle, protectedDescription)
	require.NoError(t, err)
	ordinary, err := svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: code, OrgID: targetOrg, Title: protectedTitle, Description: protectedDescription,
	}, dispatcher)
	require.NoError(t, err)

	assertSummary := func(t *testing.T, got *model.Ticket) {
		t.Helper()
		require.Equal(t, "漏洞整改", got.Title)
		require.Equal(t, "请在风险模块查看有权访问的整改事项。", got.Description)
	}
	t.Run("historical_detail_and_write_snapshot", func(t *testing.T) {
		got, err := svc.Get(ctx, linked.ID, dispatcher)
		require.NoError(t, err)
		assertSummary(t, got)
		priority := 3
		updated, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: linked.ID, Priority: &priority}, dispatcher)
		require.NoError(t, err)
		assertSummary(t, updated)
		require.Equal(t, priority, updated.Priority)
	})
	t.Run("historical_list_and_export", func(t *testing.T) {
		query := model.TicketListQuery{TypeCode: code}
		got, err := svc.List(ctx, query, dispatcher)
		require.NoError(t, err)
		require.Len(t, got.List, 2)
		require.Equal(t, ordinary.ID, got.List[0].ID)
		require.Equal(t, protectedTitle, got.List[0].Title)
		require.Equal(t, linked.ID, got.List[1].ID)
		assertSummary(t, got.List[1])
		query.Keyword = protectedTitle
		got, err = svc.List(ctx, query, dispatcher)
		require.NoError(t, err)
		require.EqualValues(t, 1, got.Total)
		require.Equal(t, ordinary.ID, got.List[0].ID)
		query.Keyword = "漏洞整改"
		count, err := svc.ExportStream(ctx, query, dispatcher, func(got *model.Ticket) error {
			assertSummary(t, got)
			require.Equal(t, linked.ID, got.ID)
			return nil
		})
		require.NoError(t, err)
		require.EqualValues(t, 1, count)
	})
	t.Run("historical_relation_candidates", func(t *testing.T) {
		scope := resource.Filter{Where: "type_code = $1", Args: []any{code}}
		got, err := svc.ticketRepo.SearchRelationCandidates(ctx, scope, "")
		require.NoError(t, err)
		require.Len(t, got, 2)
		require.Equal(t, ordinary.ID, got[0].ID)
		require.Equal(t, protectedTitle, got[0].Title)
		require.Equal(t, linked.ID, got[1].ID)
		require.Equal(t, "漏洞整改", got[1].Title)
	})
	t.Run("historical_timeout_and_deadline_events", func(t *testing.T) {
		_, err := testPool.Exec(ctx, `UPDATE tickets SET status_changed_at=now()-interval '2 hours', sla_due_at=now()-interval '1 hour' WHERE id=$1`, linked.ID)
		require.NoError(t, err)
		reg := domainevent.NewHookRegistry(nil)
		events := make(chan domainevent.DomainEvent, 2)
		reg.Subscribe(domainevent.BizTicket, "synthetic.b2.summary", domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) {
			events <- ev
		}))
		scanner := NewTimeoutScanner(testPool, reg, false, nil)
		rules := repository.BoundTicketReminderRules{TypeCode: code, DefinitionVersion: definition.DefinitionVersion}
		n, err := scanner.scanState(ctx, rules, StatusAssigned, 30, nil)
		require.NoError(t, err)
		require.Equal(t, 1, n)
		n, err = scanner.scanDeadline(ctx, rules, StatusAssigned)
		require.NoError(t, err)
		require.Equal(t, 1, n)

		var actions []domainevent.Action
		for range 2 {
			select {
			case ev := <-events:
				require.Equal(t, strconv.FormatInt(linked.ID, 10), ev.BizID)
				require.Equal(t, "漏洞整改", ev.Payload["title"])
				actions = append(actions, ev.Action)
			case <-time.After(3 * time.Second):
				t.Fatal("missing reminder event")
			}
		}
		require.ElementsMatch(t, []domainevent.Action{domainevent.TicketTimeout, domainevent.TicketDeadline}, actions)
	})
	t.Run("patch_cannot_store_or_publish_protected_copy", func(t *testing.T) {
		reg := domainevent.NewHookRegistry(nil)
		events := make(chan domainevent.DomainEvent, 1)
		reg.Subscribe(domainevent.BizTicket, "synthetic.b2.patch", domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) {
			events <- ev
		}))
		svc.SetEventRegistry(reg)
		title, description := protectedTitle, protectedDescription
		updated, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: linked.ID, Title: &title, Description: &description}, dispatcher)
		require.NoError(t, err)
		assertSummary(t, updated)
		require.NoError(t, testPool.QueryRow(ctx, `SELECT title, description FROM tickets WHERE id=$1`, linked.ID).Scan(&title, &description))
		require.Equal(t, updated.Title, title)
		require.Equal(t, updated.Description, description)
		select {
		case ev := <-events:
			require.Equal(t, []domainevent.Change{{Key: "title", NewVal: title}, {Key: "description", NewVal: description}}, ev.Payload["changes"])
		case <-time.After(3 * time.Second):
			t.Fatal("missing update event")
		}
	})
}

//go:build integration

package ticket

import (
	"context"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

func inboxBusinessFixture(t *testing.T) (*Service, *model.Ticket, int64, int64, *domainevent.HookRegistry, <-chan domainevent.DomainEvent) {
	t.Helper()
	ctx := context.Background()
	svc, creator, assignee, _, roles := setupTicket2a(t)
	roles[creator], roles[assignee] = []string{"admin"}, []string{"admin"}
	code := "inbox_business_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), `UPDATE ticket_types SET is_active=false WHERE code=$1`, code)
		require.NoError(t, err)
	})
	oid := childOrgID(t, "inbox_business")
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "public change", Priority: 2, OrgID: oid}, creator)
	require.NoError(t, err)
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, creator))
	reg := domainevent.NewHookRegistry(nil)
	NewInboxService(testPool, 90, nil).SetEventRegistry(reg)
	events := make(chan domainevent.DomainEvent, 4)
	reg.Subscribe(domainevent.BizTicket, "inbox.business.capture", domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) { events <- ev }))
	svc.SetEventRegistry(reg)
	t.Cleanup(func() { drainInboxBusiness(t, reg) })
	return svc, tk, creator, assignee, reg, events
}

func drainInboxBusiness(t *testing.T, reg *domainevent.HookRegistry) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	require.NoError(t, reg.Drain(ctx))
}

func inboxBusinessCount(t *testing.T, ticketID, recipientID int64) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT count(*) FROM notifications WHERE biz_id=$1 AND recipient_id=$2`, ticketID, recipientID).Scan(&n))
	return n
}

func TestInboxBusinessEvents_RealChanges(t *testing.T) {
	for _, kind := range []string{"update", "comment"} {
		for _, self := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/self=%v", kind, self), func(t *testing.T) {
				svc, tk, creator, assignee, reg, events := inboxBusinessFixture(t)
				actor := creator
				if self {
					actor = assignee
				}
				ctx := context.Background()
				var err error
				if kind == "update" {
					title := "updated by another person"
					_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: &title}, actor)
				} else {
					_, err = svc.CreateComment(ctx, &model.CreateCommentRequest{TicketID: tk.ID, Content: "public reply body"}, actor)
				}
				require.NoError(t, err)
				drainInboxBusiness(t, reg)
				want := 1
				if self {
					want = 0
				}
				require.Equal(t, want, inboxBusinessCount(t, tk.ID, assignee), "real service events must reach another assignee, while self actions remain quiet")
				require.Zero(t, inboxBusinessCount(t, tk.ID, creator))
				require.Len(t, events, 1)
				ev := <-events
				require.Equal(t, actor, ev.ActorID)
				require.Equal(t, strconv.FormatInt(creator, 10), ev.Payload["creator"])
				require.Equal(t, strconv.FormatInt(assignee, 10), ev.Payload["to_assignee"])
				require.NotContains(t, ev.Payload, "content", "notification event contains metadata, not reply text")
			})
		}
	}
}

func TestInboxBusinessEvents_InternalNoteStaysPrivate(t *testing.T) {
	svc, tk, creator, assignee, reg, events := inboxBusinessFixture(t)
	_, err := svc.CreateNote(context.Background(), &model.CreateNoteRequest{TicketID: tk.ID, Content: "private note"}, creator)
	require.NoError(t, err)
	drainInboxBusiness(t, reg)
	require.Zero(t, inboxBusinessCount(t, tk.ID, assignee))
	require.Empty(t, events)
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT count(*) FROM ticket_events WHERE ticket_id=$1 AND action='note'`, tk.ID).Scan(&n))
	require.Equal(t, 1, n, "internal note still retains its audit event")
}

func TestInboxBusinessEvents_RolledBackCommentDoesNotPublish(t *testing.T) {
	svc, tk, creator, assignee, reg, events := inboxBusinessFixture(t)
	ctx := context.Background()
	name := "reject_comment_" + uniqueSuffix()
	_, err := testPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'audit deferred comment failure'; END $$`, name))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER IF EXISTS %s ON ticket_comments`, name))
		require.NoError(t, err)
		_, err = testPool.Exec(context.Background(), fmt.Sprintf(`DROP FUNCTION IF EXISTS %s()`, name))
		require.NoError(t, err)
	})
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE CONSTRAINT TRIGGER %s AFTER INSERT ON ticket_comments DEFERRABLE INITIALLY DEFERRED FOR EACH ROW WHEN (NEW.ticket_id=%d) EXECUTE FUNCTION %s()`, name, tk.ID, name))
	require.NoError(t, err)
	_, err = svc.CreateComment(ctx, &model.CreateCommentRequest{TicketID: tk.ID, Content: "rolled back reply"}, creator)
	require.Error(t, err)
	drainInboxBusiness(t, reg)
	require.Zero(t, inboxBusinessCount(t, tk.ID, assignee))
	require.Empty(t, events, "failed commit must not leave an event on the bus")
	var comments, audits int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_comments WHERE ticket_id=$1`, tk.ID).Scan(&comments))
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_events WHERE ticket_id=$1 AND action='comment'`, tk.ID).Scan(&audits))
	require.Zero(t, comments)
	require.Zero(t, audits)
}

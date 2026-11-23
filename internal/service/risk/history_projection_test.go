package risk

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
)

type historyVisibility func(context.Context, int64, int64) bool

func (f historyVisibility) CanViewTicket(ctx context.Context, actorID, ticketID int64) bool {
	return f(ctx, actorID, ticketID)
}

func TestDispatchHistoryProjectsTicketOnlyForCurrentViewer(t *testing.T) {
	ctx := context.Background()
	original := []byte(`{"ticket_id":"72","ticket_ref":"synthetic-ticket","group_key":"synthetic-group","state":"dispatched"}`)
	canView := true
	viewer := historyVisibility(func(_ context.Context, actorID, ticketID int64) bool {
		require.Equal(t, int64(17), actorID)
		require.Equal(t, int64(72), ticketID)
		return canView
	})
	require.JSONEq(t, string(original), string(projectHistoryPayload(ctx, 17, "dispatch", original, viewer)))
	canView = false
	require.JSONEq(t, `{"state":"dispatched"}`, string(projectHistoryPayload(ctx, 17, "dispatch", original, viewer)))
	require.JSONEq(t, `{"state":"dispatched"}`, string(projectHistoryPayload(ctx, 17, "dispatch", original, nil)))
	require.JSONEq(t, string(original), string(projectHistoryPayload(ctx, 17, "unrelated", original, viewer)))
}

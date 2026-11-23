package risk

import (
	"context"
	"encoding/json"
	"strconv"
)

type TicketVisibility interface {
	CanViewTicket(context.Context, int64, int64) bool
}

// Risk visibility does not confer access to a linked ticket.
func projectHistoryPayload(ctx context.Context, actorID int64, event string, value []byte,
	tickets TicketVisibility) json.RawMessage {
	payload := bytesOr(value, []byte("{}"))
	if event != "dispatch" {
		return payload
	}
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(payload, &fields); err != nil {
		return json.RawMessage("{}")
	}
	var ref string
	if json.Unmarshal(fields["ticket_id"], &ref) == nil {
		id, err := strconv.ParseInt(ref, 10, 64)
		if err == nil && id > 0 && tickets != nil && tickets.CanViewTicket(ctx, actorID, id) {
			return payload
		}
	}
	delete(fields, "ticket_id")
	delete(fields, "ticket_ref")
	delete(fields, "group_key")
	return jsonBytes(fields)
}

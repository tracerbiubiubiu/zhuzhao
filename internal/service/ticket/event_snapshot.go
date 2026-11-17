package ticket

import (
	"strconv"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
)

// withTicketSnapshot uses the already-read operation snapshot. Explicit event
// facts (new status, new assignee) take precedence; consumers never reread pools.
func withTicketSnapshot(ticket *model.Ticket, ev domainevent.DomainEvent) domainevent.DomainEvent {
	payload := domainevent.Payload{"title": ticket.Title, "type_code": ticket.TypeCode, "priority": ticket.Priority, "status": ticket.Status}
	if ticket.HumanKey != "" {
		payload["human_key"] = ticket.HumanKey
	}
	if ticket.AssignedTo != nil {
		payload["to_assignee"] = strconv.FormatInt(*ticket.AssignedTo, 10)
	}
	for key, value := range ev.Payload {
		payload[key] = value
	}
	ev.Payload = payload
	return ev
}

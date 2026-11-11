package ticket

import (
	"github.com/stretchr/testify/assert"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"testing"
)

func TestNotificationCreator_RecipientBoundaries(t *testing.T) {
	ev := domainevent.DomainEvent{BizType: domainevent.BizTicket, ActorID: 99, Payload: domainevent.Payload{"actor": "99", "creator": "42", "to_assignee": "100", "cc_users": "101,102"}}
	assert.Equal(t, []string{"42"}, resolveRecipientsFromRule(ev, "creator", ""))
	assert.Equal(t, []string{"100"}, resolveRecipientsFromRule(ev, "assignee", ""))
	assert.Equal(t, []string{"101", "102"}, resolveRecipientsFromRule(ev, "cc", ""))
	assert.Equal(t, []string{"201", "202"}, resolveRecipientsFromRule(ev, "fixed", "201,202"))
	delete(ev.Payload, "creator")
	assert.Empty(t, resolveRecipientsFromRule(ev, "creator", ""), "ticket missing creator must not fall back to the actor")
	ev.BizType = domainevent.BizOps
	assert.Equal(t, []string{"99"}, resolveRecipientsFromRule(ev, "creator", ""), "ops action responsibility retains its existing actor meaning")
}

func TestNotificationCreator_IdentityDoesNotMutateSharedPayload(t *testing.T) {
	original := domainevent.Payload{"title": "snapshot"}
	ev := withTicketIdentity(domainevent.DomainEvent{ActorID: 99, Payload: original}, 42)
	assert.Equal(t, "42", ev.Payload["creator"])
	assert.Equal(t, "99", ev.Payload["actor"])
	assert.NotContains(t, original, "creator")
	assert.NotContains(t, original, "actor")
	scanner := withTicketIdentity(domainevent.DomainEvent{}, 42)
	assert.Equal(t, "42", scanner.Payload["creator"])
	assert.NotContains(t, scanner.Payload, "actor", "maintenance scanner has no human actor")
}

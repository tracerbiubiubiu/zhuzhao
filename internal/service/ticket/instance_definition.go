package ticket

import (
	"context"
	"errors"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func (s *Service) resolveTicketDefinition(ctx context.Context, ticket *model.Ticket) (*model.TicketType, []*model.TicketTypeField, *model.TicketDefinitionRef, error) {
	snapshot, ref, err := s.ticketRepo.BoundTicketDefinition(ctx, ticket.TypeCode, ticket.DefinitionVersion)
	if err != nil {
		return nil, nil, nil, err
	}
	return &snapshot.Type, snapshot.Fields, ref, nil
}

func (s *Service) describeTicketDefinition(ctx context.Context, ticket *model.Ticket) (*model.Ticket, error) {
	_, _, ref, err := s.resolveTicketDefinition(ctx, ticket)
	if errors.Is(err, errcode.ErrServiceUnavailable) {
		ticket.DefinitionResolution = "degraded"
		return ticket, nil
	}
	if err != nil {
		return nil, err
	}
	ticket.DefinitionRef = ref
	ticket.DefinitionResolution = "exact"
	return ticket, nil
}

// requireTicketDefinition checks the binding of an already locked ticket for
// mutations that do not otherwise consume any rules.
func (s *Service) requireTicketDefinition(ctx context.Context, id int64) error {
	ticket, err := s.ticketRepo.GetByID(ctx, id)
	if err != nil {
		return err
	}
	_, _, _, err = s.resolveTicketDefinition(ctx, ticket)
	return err
}

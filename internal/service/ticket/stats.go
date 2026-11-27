package ticket

import (
	"context"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func (s *Service) Stats(ctx context.Context, q model.TicketListQuery, actor int64, groupBy string) (*model.TicketStats, error) {
	filter, err := s.resolveListFilter(ctx, actor)
	if err != nil {
		return nil, err
	}
	return s.ticketRepo.TicketStats(ctx, filter, q, groupBy, time.Now().UTC().Truncate(time.Microsecond))
}

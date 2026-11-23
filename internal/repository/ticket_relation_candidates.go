package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

func (r *TicketRepo) SearchRelationCandidates(ctx context.Context, filter resource.Filter, keyword string) ([]model.TicketRelationCandidate, error) {
	if filter.Where == "" {
		return nil, fmt.Errorf("ticket relation candidates: missing authorization predicate")
	}
	where, args := buildTicketWhere(filter, model.TicketListQuery{Keyword: keyword})
	rows, err := IAMDB(ctx, r.db).Query(ctx, `SELECT id, `+TicketSummaryTitleSQL+`, status FROM tickets`+where+` ORDER BY id DESC LIMIT 20`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowToStructByPos[model.TicketRelationCandidate])
}

package repository

import (
	"context"
	"fmt"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

// TicketStats groups the complete visible selection; pagination does not affect totals.
func (r *TicketRepo) TicketStats(ctx context.Context, filter resource.Filter, q model.TicketListQuery, groupBy string, asOf time.Time) (*model.TicketStats, error) {
	if filter.Where == "" && !filter.Unscoped {
		return nil, fmt.Errorf("ticket statistics: missing scope filter")
	}
	switch groupBy {
	case "status", "priority", "type_code":
	default:
		return nil, errcode.ErrInvalidParams
	}
	where, args := buildTicketWhere(filter, q)
	args = append(args, asOf)
	// groupBy is allowlisted above. Bind the same instant for every bucket.
	rows, err := IAMDB(ctx, r.db).Query(ctx, `SELECT `+groupBy+`::text, count(*), count(*) FILTER (WHERE active),
        count(*) FILTER (WHERE active AND sla_due_at < `+fmt.Sprintf("$%d", len(args))+`)
        FROM (SELECT `+groupBy+`,sla_due_at,status NOT IN ('closed','rejected','cancelled') AS active FROM tickets`+where+`) selected
        GROUP BY 1 ORDER BY 1`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := model.TicketStats{AsOf: asOf, Items: []model.TicketStatsItem{}}
	for rows.Next() {
		var item model.TicketStatsItem
		if err := rows.Scan(&item.Key, &item.Count, &item.InProgressCount, &item.OverdueCount); err != nil {
			return nil, err
		}
		if item.InProgressCount > 0 {
			rate := float64(item.OverdueCount) / float64(item.InProgressCount)
			item.OverdueRate = &rate
		}
		result.Items = append(result.Items, item)
		result.Total += item.Count
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return &result, nil
}

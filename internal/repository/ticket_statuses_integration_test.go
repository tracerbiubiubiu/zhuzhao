//go:build integration

package repository_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// 首页待办的集合语义必须同时约束分页列表、导出计数和导出流，不能突破L2。
func TestTicketStatuses_ListAndExportRespectScope(t *testing.T) {
	ctx := context.Background()
	repo := repository.NewTicketRepo(testPool)
	var visible, expected []int64
	for _, status := range []string{"assigned", "in_progress", "pending_verify", "closed", "rejected"} {
		id := conflictTicket(t, status)
		visible = append(visible, id)
		if status == "assigned" || status == "in_progress" {
			expected = append(expected, id)
		}
	}
	conflictTicket(t, "assigned") // 同状态但scope不可见，必须不返回
	filter := resource.Filter{Where: "id = ANY($1::bigint[])", Args: []any{visible}}
	q := model.TicketListQuery{Statuses: []string{"assigned", "in_progress"}, Page: 1, PageSize: 20}
	rows, total, err := repo.List(ctx, filter, q)
	require.NoError(t, err)
	require.EqualValues(t, 2, total)
	var ids []int64
	for _, row := range rows {
		ids = append(ids, row.ID)
	}
	require.ElementsMatch(t, expected, ids)
	count, err := repo.CountByFilter(ctx, filter, q)
	require.NoError(t, err)
	require.Equal(t, total, count)
	var exported []int64
	require.NoError(t, repo.StreamRowsByFilter(ctx, filter, q, func(row *model.Ticket) error {
		exported = append(exported, row.ID)
		return nil
	}))
	require.ElementsMatch(t, expected, exported)
	_, total, err = repo.List(ctx, filter, model.TicketListQuery{Status: "closed"})
	require.NoError(t, err)
	require.EqualValues(t, 1, total, "单状态兼容")
	_, total, err = repo.List(ctx, filter, model.TicketListQuery{})
	require.NoError(t, err)
	require.EqualValues(t, 5, total, "省略状态仍保留原全部可见语义")
}

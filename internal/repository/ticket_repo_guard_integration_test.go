//go:build integration

package repository_test

import (
	"context"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// IW4 集成侧：显式 Unscoped 豁免（admin bypass / ticket_scope=all 的语义）在真实
// DB 上可正常查询，零值 Filter 仍被哨兵拒绝——守卫不改变存量合法路径的行为。
func TestTicketRepoList_Iw4Guard(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()

	t.Run("explicit unscoped passes", func(t *testing.T) {
		tickets, total, err := repo.List(ctx, resource.Filter{Unscoped: true}, model.TicketListQuery{Page: 1, PageSize: 10})
		require.NoError(t, err)
		require.GreaterOrEqual(t, total, int64(0))
		require.LessOrEqual(t, len(tickets), 10)
	})

	t.Run("zero-value filter rejected", func(t *testing.T) {
		_, _, err := repo.List(ctx, resource.Filter{}, model.TicketListQuery{Page: 1, PageSize: 10})
		require.Error(t, err)
		require.True(t, strings.Contains(err.Error(), "scope filter"), "错误应指明缺少 scope 过滤，got: %v", err)
	})
}

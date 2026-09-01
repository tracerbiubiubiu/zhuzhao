package repository

import (
	"context"
	"strings"
	"testing"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

// IW4 守护：repo.List 入口 fail-closed——零值 Filter（无谓词且未豁免）必须被拒绝，
// 而不是静默全量。哨兵在任何 DB 访问之前返回，nil 池即可测。
func TestTicketRepoList_RejectsFilterWithoutScope(t *testing.T) {
	repo := NewTicketRepo(nil)
	tickets, total, err := repo.List(context.Background(), resource.Filter{}, model.TicketListQuery{Page: 1, PageSize: 10})
	if err == nil {
		t.Fatalf("零值 Filter 应被哨兵拒绝，却返回了 %d 行", total)
	}
	if !strings.Contains(err.Error(), "scope filter") {
		t.Fatalf("错误信息应指明缺少 scope 过滤，got: %v", err)
	}
	if tickets != nil || total != 0 {
		t.Fatalf("拒绝时不应返回数据：tickets=%v total=%d", tickets, total)
	}
}

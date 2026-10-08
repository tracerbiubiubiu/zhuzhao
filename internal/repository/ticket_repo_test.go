package repository

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

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

// ticketListConds 条件拼装（工单列表新增筛选批）：$n 占位与 args 严格同序、
// D2-21 ILIKE 转义、纯数字 keyword OR ID 精确匹配、时间闭开区间——零 DB 纯函数测
func TestTicketListConds(t *testing.T) {
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	toEx := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC) // to 日次日 00:00（闭开上界）
	uid := int64(7)

	t.Run("空查询无条件", func(t *testing.T) {
		conds, args := ticketListConds(model.TicketListQuery{})
		if len(conds) != 0 || len(args) != 0 {
			t.Fatalf("应无条件无参数，got conds=%v args=%v", conds, args)
		}
	})

	t.Run("存量四参条件与占位同序", func(t *testing.T) {
		conds, args := ticketListConds(model.TicketListQuery{
			TypeCode: "net", Status: "open", Priority: intPtr(2), AssigneeID: &uid,
		})
		want := []string{"type_code = $1", "status = $2", "priority = $3", "assigned_to = $4"}
		for i, w := range want {
			if conds[i] != w {
				t.Fatalf("conds[%d] = %q, want %q", i, conds[i], w)
			}
		}
		if len(args) != 4 {
			t.Fatalf("应 4 个参数，got %d", len(args))
		}
	})

	t.Run("keyword 转义 ILIKE 元字符（D2-21）", func(t *testing.T) {
		conds, args := ticketListConds(model.TicketListQuery{Keyword: `50%_ok\`})
		if len(conds) != 1 || conds[0] != `title ILIKE $1 ESCAPE '\'` {
			t.Fatalf("unexpected conds: %v", conds)
		}
		if args[0] != `%50\%\_ok\\%` {
			t.Fatalf("pattern 未转义: %q", args[0])
		}
	})

	t.Run("纯数字 keyword OR 工单 ID 精确匹配", func(t *testing.T) {
		conds, args := ticketListConds(model.TicketListQuery{Keyword: "123"})
		if len(conds) != 1 || conds[0] != `(title ILIKE $1 ESCAPE '\' OR id = $2)` {
			t.Fatalf("unexpected conds: %v", conds)
		}
		if args[1] != int64(123) {
			t.Fatalf("ID 参数应为 int64 123，got %T %v", args[1], args[1])
		}
	})

	t.Run("含非数字字符不带 ID 分支", func(t *testing.T) {
		conds, args := ticketListConds(model.TicketListQuery{Keyword: "1a"})
		if len(conds) != 1 || strings.Contains(conds[0], "id =") {
			t.Fatalf("非纯数字不应带 ID 条件: %v", conds)
		}
		if len(args) != 1 {
			t.Fatalf("应 1 个参数，got %d", len(args))
		}
	})

	t.Run("时间区间闭开边界", func(t *testing.T) {
		conds, args := ticketListConds(model.TicketListQuery{CreatedFromAt: &from, CreatedToAt: &toEx})
		if conds[0] != "created_at >= $1" || conds[1] != "created_at < $2" {
			t.Fatalf("unexpected conds: %v", conds)
		}
		if args[0] != from || args[1] != toEx {
			t.Fatalf("区间参数不符: %v %v", args[0], args[1])
		}
	})

	t.Run("混合条件占位连续无跳号", func(t *testing.T) {
		conds, args := ticketListConds(model.TicketListQuery{
			Status: "open", CreatedBy: &uid, Keyword: "网络", CreatedFromAt: &from, CreatedToAt: &toEx,
		})
		if len(conds) != len(args) {
			t.Fatalf("conds 数 %d 与 args 数 %d 不一致", len(conds), len(args))
		}
		for i, c := range conds {
			if !strings.Contains(c, fmt.Sprintf("$%d", i+1)) {
				t.Fatalf("conds[%d]=%q 未引用 $%d", i, c, i+1)
			}
		}
	})
}

func intPtr(i int) *int { return &i }

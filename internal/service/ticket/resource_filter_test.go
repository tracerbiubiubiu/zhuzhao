package ticket

import (
	"context"
	"strings"
	"testing"
)

// fakeResolver 固定返回预置 ResolvedScope，供 GetFilter 单测使用。
type fakeResolver struct {
	scope *ResolvedScope
}

func (f *fakeResolver) ReadAnchorPaths(ctx context.Context, userID int64) ([]string, error) {
	return f.scope.ReadPaths(), nil
}

func (f *fakeResolver) ResolveScope(ctx context.Context, userID int64) (*ResolvedScope, error) {
	return f.scope, nil
}

// IW4：ticket_scope=all 的「策略性全量」必须显式携带 Unscoped，
// 与 admin bypass 同构（零值 Filter{} 已被 repo 哨兵拒绝）。
func TestGetFilter_AllScopeExplicitUnscoped(t *testing.T) {
	res := NewResource(nil, &fakeResolver{scope: &ResolvedScope{AllScope: true}}, nil)
	filter, err := res.GetFilter(context.Background(), 1, "list")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if !filter.Unscoped {
		t.Fatalf("AllScope 分支应显式 Unscoped，got %+v", filter)
	}
	if filter.Where != "" {
		t.Fatalf("AllScope 不应携带谓词，got %q", filter.Where)
	}
}

// 常规 scope（锚点 ∪ group 子树）仍产出谓词且不带豁免——哨兵不得误伤正常过滤。
func TestGetFilter_NormalScopeKeepsPredicate(t *testing.T) {
	res := NewResource(nil, &fakeResolver{scope: &ResolvedScope{
		AnchorPaths: []string{"corp.ent"},
		ScopePaths:  []string{"corp.dept"},
	}}, nil)
	filter, err := res.GetFilter(context.Background(), 1, "list")
	if err != nil {
		t.Fatalf("GetFilter: %v", err)
	}
	if filter.Unscoped {
		t.Fatalf("常规 scope 不应豁免，got %+v", filter)
	}
	if !strings.Contains(filter.Where, "created_by") || !strings.Contains(filter.Where, "org_path <@") {
		t.Fatalf("谓词应包含属主/锚点条件，got %q", filter.Where)
	}
}

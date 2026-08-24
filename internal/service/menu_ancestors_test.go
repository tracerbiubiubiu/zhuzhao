package service

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// D2-25 守护：includeMenuAncestors 对 DB 脏数据成环（A→B→A）不死循环，
// 且正常祖先补全语义不受影响（与 buildMenuTree 的孤儿策略行为对齐）
func TestIncludeMenuAncestors(t *testing.T) {
	// 正常链：btn(3) → page(2) → dir(1)，勾选 btn 应补全两个祖先
	dir := &model.Menu{ID: 1, Code: "system", MenuType: 1}
	page := &model.Menu{ID: 2, Code: "system_user", MenuType: 2, ParentID: &dir.ID}
	btn := &model.Menu{ID: 3, Code: "user_delete", MenuType: 3, ParentID: &page.ID}

	out := includeMenuAncestors([]*model.Menu{btn}, []*model.Menu{dir, page, btn})
	require.Len(t, out, 3, "按钮 + 补全的页面与目录")

	// 环：A.parent=B、B.parent=A——修复前链式上溯死循环
	a := &model.Menu{ID: 10, Code: "cycle_a", MenuType: 1}
	b := &model.Menu{ID: 11, Code: "cycle_b", MenuType: 1}
	a.ParentID = &b.ID
	b.ParentID = &a.ID

	done := make(chan []*model.Menu, 1)
	go func() { done <- includeMenuAncestors([]*model.Menu{a}, []*model.Menu{a, b}) }()
	select {
	case got := <-done:
		assert.Len(t, got, 2, "环内两节点各收录一次后终止")
	case <-timeoutAfterSec(5):
		t.Fatal("includeMenuAncestors 对环形脏数据死循环（D2-25 回归）")
	}

	// 自环：A.parent=A
	self := &model.Menu{ID: 12, Code: "cycle_self", MenuType: 1}
	selfID := self.ID
	self.ParentID = &selfID
	out = includeMenuAncestors([]*model.Menu{self}, []*model.Menu{self})
	assert.Len(t, out, 1, "自环节点只收录一次")

	// 孤儿：父不在 all 内——收录自身即止
	orphanParent := int64(999)
	orphan := &model.Menu{ID: 13, Code: "orphan", MenuType: 2, ParentID: &orphanParent}
	out = includeMenuAncestors([]*model.Menu{orphan}, []*model.Menu{orphan})
	assert.Len(t, out, 1)
}

func timeoutAfterSec(sec int) <-chan time.Time {
	return time.After(time.Duration(sec) * time.Second)
}

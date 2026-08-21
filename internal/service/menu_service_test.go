package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// 回归测试：buildMenuTree 曾因 map 遍历顺序随机丢失孙子节点
// （父节点先于孙节点处理时，嵌入父 Children 的是尚未挂子的过期值拷贝）。
// go test 多次运行（-count）下必须全部稳定通过。
func TestBuildMenuTree(t *testing.T) {
	root := &model.Menu{ID: 1, Code: "system", Name: "系统管理", MenuType: 1, SortOrder: 1}
	pageA := &model.Menu{ID: 2, Code: "system_user", Name: "用户管理", MenuType: 2, ParentID: &root.ID, SortOrder: 1}
	pageB := &model.Menu{ID: 3, Code: "system_role", Name: "角色管理", MenuType: 2, ParentID: &root.ID, SortOrder: 2}
	btnA1 := &model.Menu{ID: 4, Code: "user_delete", Name: "删除用户", MenuType: 3, ParentID: &pageA.ID, Permission: "user:delete", SortOrder: 1}
	btnA2 := &model.Menu{ID: 5, Code: "user_create", Name: "新建用户", MenuType: 3, ParentID: &pageA.ID, Permission: "user:create", SortOrder: 2}
	orphan := &model.Menu{ID: 99, Code: "orphan", Name: "孤儿", MenuType: 1, ParentID: func() *int64 { id := int64(999); return &id }(), SortOrder: 9}

	menus := []*model.Menu{pageB, btnA2, root, btnA1, pageA, orphan}
	tree := buildMenuTree(menus)

	require.Len(t, tree, 2, "根节点 + 父不存在的孤儿节点")

	// 根：目录含两个页面，按 sort_order 排序
	require.Equal(t, int64(1), tree[0].ID)
	require.Len(t, tree[0].Children, 2)
	assert.Equal(t, int64(2), tree[0].Children[0].ID)
	assert.Equal(t, int64(3), tree[0].Children[1].ID)

	// 三层完整性：页面下挂两个按钮（曾随机丢失的孙子节点）
	require.Len(t, tree[0].Children[0].Children, 2, "孙子节点（按钮）不得丢失")
	assert.Equal(t, int64(4), tree[0].Children[0].Children[0].ID)
	assert.Equal(t, int64(5), tree[0].Children[0].Children[1].ID)

	// 孤儿按根节点处理
	assert.Equal(t, int64(99), tree[1].ID)
}

func TestBuildMenuTreeEmpty(t *testing.T) {
	assert.Equal(t, []model.Menu{}, buildMenuTree(nil))
}

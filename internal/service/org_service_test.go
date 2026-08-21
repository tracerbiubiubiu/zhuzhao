package service

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestBuildOrgTree(t *testing.T) {
	root := &model.Organization{ID: 1, Code: "root", SortOrder: 1}
	deptB := &model.Organization{ID: 3, Code: "dept_b", ParentID: &root.ID, SortOrder: 2}
	deptA := &model.Organization{ID: 2, Code: "dept_a", ParentID: &root.ID, SortOrder: 1}
	teamA1 := &model.Organization{ID: 4, Code: "team_a1", ParentID: &deptA.ID, SortOrder: 1}
	orphan := &model.Organization{ID: 99, Code: "orphan", ParentID: func() *int64 { id := int64(999); return &id }(), SortOrder: 9}

	tree := buildOrgTree([]*model.Organization{deptB, root, teamA1, deptA, orphan})

	require.Len(t, tree, 2, "根节点 + 父不存在的孤儿节点")

	// 根节点：sort_order 升序的子节点
	require.Equal(t, int64(1), tree[0].ID)
	require.Len(t, tree[0].Children, 2)
	assert.Equal(t, int64(2), tree[0].Children[0].ID, "sort_order=1 的 dept_a 在前")
	assert.Equal(t, int64(3), tree[0].Children[1].ID, "sort_order=2 的 dept_b 在后")
	// 孙节点挂在 dept_a 下
	require.Len(t, tree[0].Children[0].Children, 1)
	assert.Equal(t, int64(4), tree[0].Children[0].Children[0].ID)

	// 孤儿节点（父 ID 不存在）按根节点处理，不丢节点
	assert.Equal(t, int64(99), tree[1].ID)
	assert.Empty(t, tree[1].Children)
}

func TestBuildOrgTreeEmpty(t *testing.T) {
	assert.Equal(t, []*model.Organization{}, buildOrgTree(nil))
}

// TestBuildOrgTreeNoSideEffect 验证：buildOrgTree 不会修改原始输入的 Children，
// 且每个节点都是独立副本（原对象保持"光杆"平铺态）。
func TestBuildOrgTreeNoSideEffect(t *testing.T) {
	root := &model.Organization{ID: 1, SortOrder: 1}
	child := &model.Organization{ID: 2, ParentID: &root.ID, SortOrder: 1}

	// 记录原始状态，便于事后对照
	require.Nil(t, root.Children, "前置：root 原本没有 Children")
	require.Nil(t, child.Children, "前置：child 原本没有 Children")

	tree := buildOrgTree([]*model.Organization{root, child})
	require.Len(t, tree, 1)
	require.Len(t, tree[0].Children, 1, "树里 child 已挂到 root 下")

	// 原始输入必须保持原样（隔离性：先 new 再拷贝，不原地挂载）
	assert.Nil(t, root.Children, "原始 root 不应被写入 Children")
	assert.Nil(t, child.Children, "原始 child 不应被写入 Children")
	// 返回的节点与原始对象是不同地址（独立副本）
	assert.NotSame(t, root, tree[0], "返回的节点应是新建的独立副本，而非原对象")
}

// TestBuildOrgTreeSameSortOrderByID 验证：同层 sort_order 相同时按 ID 升序。
func TestBuildOrgTreeSameSortOrderByID(t *testing.T) {
	root := &model.Organization{ID: 1, SortOrder: 1}
	// 两个孩子 sort_order 都是 5，应严格按 ID：3 在 7 前
	c1 := &model.Organization{ID: 7, ParentID: &root.ID, SortOrder: 5}
	c2 := &model.Organization{ID: 3, ParentID: &root.ID, SortOrder: 5}

	// 故意打乱传入顺序
	tree := buildOrgTree([]*model.Organization{root, c1, c2})
	require.Len(t, tree, 1)
	require.Len(t, tree[0].Children, 2)
	assert.Equal(t, int64(3), tree[0].Children[0].ID, "相同 sort_order 时 ID 小的在前")
	assert.Equal(t, int64(7), tree[0].Children[1].ID, "相同 sort_order 时 ID 大的在后")
}

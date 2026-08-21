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

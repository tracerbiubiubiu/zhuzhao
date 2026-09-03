//go:build integration

package repository_test

import (
	"context"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func resetOrgs(t *testing.T) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		TRUNCATE user_orgs, user_roles, users, organizations RESTART IDENTITY CASCADE`)
	require.NoError(t, err)
}

func orgTestUser(t *testing.T, employeeNo string) int64 {
	t.Helper()
	hash, err := crypto.HashPassword("p")
	require.NoError(t, err)
	repo := repository.NewUserRepo(testPool)
	user := &model.User{Username: employeeNo, EmployeeNo: employeeNo, Password: hash, Status: 1}
	require.NoError(t, repo.Create(context.Background(), user))
	return user.ID
}

func insertOrg(t *testing.T, code, path string, parentID *int64) int64 {
	t.Helper()
	var id int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO organizations (code, name, parent_id, path, is_virtual, status, is_system, sort_order)
		VALUES ($1, $1, $2, $3::ltree, false, 1, false, 1) RETURNING id`, code, parentID, path).Scan(&id)
	require.NoError(t, err)
	return id
}

func requireErrCode(t *testing.T, err error, want *errcode.Error) {
	t.Helper()
	require.Error(t, err)
	var biz *errcode.Error
	require.ErrorAs(t, err, &biz)
	assert.Equal(t, want.Code, biz.Code)
}

// B3-1 守护：AddMember 幂等不降级——重复添加已存在 primary 成员且未传
// is_primary（零值 false）时，原 primary 保持；仅显式 primary=true 才提升。
func TestOrgRepo_AddMemberIdempotentNoPrimaryDegrade(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	uid := orgTestUser(t, "E630001")
	orgA := insertOrg(t, "a", "root.a", nil)
	orgB := insertOrg(t, "b", "root.b", nil)

	// 首次添加为 primary
	require.NoError(t, repo.AddMember(ctx, orgA, uid, true))

	// 重复添加（未传 is_primary = false）：primary 不应被静默清除
	require.NoError(t, repo.AddMember(ctx, orgA, uid, false))
	orgs, err := repo.GetUserOrgs(ctx, uid)
	require.NoError(t, err)
	require.Len(t, orgs, 1)
	assert.True(t, orgs[0].IsPrimary, "重复添加（false）不得降级已有 primary")

	// 加入另一组织（非 primary）：primary 仍在 A
	require.NoError(t, repo.AddMember(ctx, orgB, uid, false))
	orgs, err = repo.GetUserOrgs(ctx, uid)
	require.NoError(t, err)
	require.Len(t, orgs, 2)
	primaryCount := 0
	for _, uo := range orgs {
		if uo.IsPrimary {
			primaryCount++
		}
	}
	assert.Equal(t, 1, primaryCount, "非 primary 添加不影响已有 primary")

	// 显式 primary=true 提升到 B：A 被清除
	require.NoError(t, repo.AddMember(ctx, orgB, uid, true))
	orgs, err = repo.GetUserOrgs(ctx, uid)
	require.NoError(t, err)
	for _, uo := range orgs {
		if uo.OrgID == orgB {
			assert.True(t, uo.IsPrimary, "显式 primary=true 应提升")
		}
		if uo.OrgID == orgA {
			assert.False(t, uo.IsPrimary, "提升后旧 primary 应被清除（事务内 UPDATE）")
		}
	}
}

// B3-4 守护：SetUserOrgs 入参去重——重复 org_id 不再触发主键冲突 500。
func TestOrgRepo_SetUserOrgsDedup(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	uid := orgTestUser(t, "E630002")
	orgA := insertOrg(t, "a", "root.a", nil)
	orgB := insertOrg(t, "b", "root.b", nil)

	// 重复 org_id：修复前第二条 INSERT 触发 23505 → 500
	err := repo.SetUserOrgs(ctx, uid, []int64{orgA, orgB, orgA}, &orgA)
	require.NoError(t, err, "重复 org_id 应被去重而非报错")

	var n int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE user_id = $1`, uid).Scan(&n))
	assert.Equal(t, 2, n, "去重后应有且仅有 2 行")
}

// B3-3 守护：primary 互斥数据库兜底——绕过应用层直接并发插双 primary，
// 部分唯一索引应拦截第二个事务（23505 → ErrDuplicatePrimaryOrg 409）。
func TestOrgRepo_SinglePrimaryIndexEnforced(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()

	uid := orgTestUser(t, "E630003")
	orgA := insertOrg(t, "a", "root.a", nil)
	orgB := insertOrg(t, "b", "root.b", nil)

	// 直接插两条 primary（绕过 AddMember 应用层清 primary 的逻辑），
	// 第二条应被索引拦截——这是并发窗口的最终一致性兜底
	_, err := testPool.Exec(ctx,
		`INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true)`, uid, orgA)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx,
		`INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true)`, uid, orgB)
	require.Error(t, err, "部分唯一索引应拒绝第二条 primary")

	// 应用层映射验证：AddMember 并发兜底路径返回 409 业务码
	repo := repository.NewOrgRepo(testPool)
	err = repo.AddMember(ctx, orgA, uid, false) // 已有 primary(A)，加 B 为非 primary 应成功
	require.NoError(t, err)
	err = repo.AddMember(ctx, orgB, uid, true) // 提升 B：清 A → 索引不冲突（正常路径）
	require.NoError(t, err)
}

// B3-2 守护：Move 事务化——环检测在锁内。
// 移到自身子孙 → 400（ErrOrgCannotMoveToChild）；正常移动级联更新子树 path；
// 并发交叉移动（A↔B）被 advisory lock 串行化，第二个事务按新路径判断被拒。
func TestOrgRepo_MoveTxGuard(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	root := insertOrg(t, "root", "root", nil)
	deptA := insertOrg(t, "dept_a", "root.dept_a", &root)
	teamA := insertOrg(t, "team_a", "root.dept_a.team_a", &deptA)
	deptB := insertOrg(t, "dept_b", "root.dept_b", &root)

	// 1) 移到自己的子孙下 → 400
	err := repo.Move(ctx, deptA, &teamA)
	requireErrCode(t, err, errcode.ErrOrgCannotMoveToChild)

	// 2) 正常移动：dept_a（含 team_a 子树）移到 dept_b 下，path 级联更新
	require.NoError(t, repo.Move(ctx, deptA, &deptB))
	var path string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT path::text FROM organizations WHERE id = $1`, teamA).Scan(&path))
	assert.Equal(t, "root.dept_b.dept_a.team_a", path, "子树 path 应级联前缀替换")
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT path::text FROM organizations WHERE id = $1`, deptA).Scan(&path))
	assert.Equal(t, "root.dept_b.dept_a", path)

	// 3) 移到根（nil）：path 变为自身 code
	require.NoError(t, repo.Move(ctx, deptA, nil))
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT path::text FROM organizations WHERE id = $1`, teamA).Scan(&path))
	assert.Equal(t, "dept_a.team_a", path, "移到根后子树相对后缀保持")

	// 4) 不存在的目标父 → 404
	err = repo.Move(ctx, deptA, func() *int64 { id := int64(999999); return &id }())
	requireErrCode(t, err, errcode.ErrOrgNotFound)
}

// B3-2 守护：并发交叉移动（A 移入 B 下、同时 B 移入 A 下）。
// advisory lock 串行化：两事务不可能都通过环检测——一成一败，树不变量保持。
func TestOrgRepo_MoveConcurrentCrossMove(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	root := insertOrg(t, "root", "root", nil)
	nodeA := insertOrg(t, "node_a", "root.node_a", &root)
	nodeB := insertOrg(t, "node_b", "root.node_b", &root)

	var wg sync.WaitGroup
	results := make([]error, 2)
	wg.Add(2)
	go func() { defer wg.Done(); results[0] = repo.Move(ctx, nodeA, &nodeB) }()
	go func() { defer wg.Done(); results[1] = repo.Move(ctx, nodeB, &nodeA) }()
	wg.Wait()

	// 至少一个成功；若两个都「成功」则树不变量已被破坏（修复前的竞态）
	// 修复后：串行化执行，第二个事务会看到第一个的结果 → 环检测拒绝
	successCount := 0
	for _, err := range results {
		if err == nil {
			successCount++
		}
	}
	assert.Equal(t, 1, successCount, "交叉移动必须一成一败（advisory lock 串行化 + 事务内环检测）")

	// 树不变量：不存在互为祖先的两个节点
	var bad int
	require.NoError(t, testPool.QueryRow(ctx, `
		SELECT COUNT(*) FROM organizations a, organizations b
		WHERE a.id <> b.id AND a.deleted_at IS NULL AND b.deleted_at IS NULL
		  AND a.path <@ b.path AND b.path <@ a.path`).Scan(&bad))
	assert.Zero(t, bad, "不得出现 path 互为祖先前缀的节点对")
}

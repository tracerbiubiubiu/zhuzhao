//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// P2-1 回归：Delete 认领（organizations FOR UPDATE）× AddMember 插入（FOR SHARE）
// 交错并发——双向各 25 轮，断言零残留（不存在「deleted_at 非空的组织仍挂 user_orgs 成员行」）。
//
// 修复前 Delete 守卫无锁（注释自称 FOR UPDATE 实无）、AddMember 不取 org 锁，窗口内
// 插入的成员行随组织软删遗留。修复后两侧锁档互斥：无论谁先认领，
//   - AddMember 先 → Delete COUNT 看到成员 → ErrOrgHasMembers（组织存活）；
//   - Delete 先 → AddMember 复核 deleted_at 非空 → ErrOrgNotFound（成员不落库）。
func TestOrgRepo_DeleteAddMemberConcurrencyNoResidual(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	const roundsEach = 25
	idx := 0
	for _, deleteFirst := range []bool{true, false} {
		for r := 0; r < roundsEach; r++ {
			code := fmt.Sprintf("race_%d", idx)
			orgID := insertOrg(t, code, "root."+code, nil)
			uid := orgTestUser(t, fmt.Sprintf("E71%05d", idx))

			var wg sync.WaitGroup
			wg.Add(2)
			var delErr, addErr error
			delFn := func() { defer wg.Done(); delErr = repo.Delete(ctx, orgID) }
			addFn := func() { defer wg.Done(); addErr = repo.AddMember(ctx, orgID, uid, false) }
			start := make(chan struct{})
			launch := func() {
				<-start
				if deleteFirst {
					delFn()
				} else {
					addFn()
				}
			}
			launch2 := func() {
				<-start
				if deleteFirst {
					addFn()
				} else {
					delFn()
				}
			}
			go launch()
			go launch2()
			close(start)
			wg.Wait()

			var residual int
			require.NoError(t, testPool.QueryRow(ctx, `
				SELECT COUNT(*) FROM user_orgs uo
				JOIN organizations o ON o.id = uo.org_id
				WHERE o.deleted_at IS NOT NULL`).Scan(&residual))
			require.Zerof(t, residual,
				"第 %d 轮（deleteFirst=%v）出现「软删组织挂成员行」残留（Delete=%v AddMember=%v）",
				idx, deleteFirst, delErr, addErr)

			idx++
		}
	}
}

// P2-1 落点覆盖：软删组织在 4 条用户-组织写入落点上都必须被守卫拦截且零写入；
// 同时正向对照存活组织的同批落点仍正常写入（守卫不误伤）。
//  1. AddMember
//  2. AddMemberWithRole
//  3. SetUserOrgs（内部经 SetUserOrgsTx）
//  4. SetOwnersTx
func TestOrgRepo_MemberInsertPathsGuardDeletedOrg(t *testing.T) {
	resetOrgs(t)
	ctx := context.Background()
	repo := repository.NewOrgRepo(testPool)

	uid := orgTestUser(t, "E730001")
	dead := insertOrg(t, "dead", "root.dead", nil)
	require.NoError(t, repo.Delete(ctx, dead), "空组织应可软删")

	// 落点 1：AddMember
	requireErrCode(t, repo.AddMember(ctx, dead, uid, false), errcode.ErrOrgNotFound)
	// 落点 2：AddMemberWithRole
	requireErrCode(t, repo.AddMemberWithRole(ctx, dead, uid, false, "member", "assigned"), errcode.ErrOrgNotFound)
	// 落点 3：SetUserOrgs → SetUserOrgsTx
	requireErrCode(t, repo.SetUserOrgs(ctx, uid, []int64{dead}, nil), errcode.ErrOrgNotFound)
	// 落点 4：SetOwnersTx
	requireErrCode(t, repo.RunInTx(ctx, func(tx pgx.Tx) error {
		return repo.SetOwnersTx(ctx, tx, dead, []int64{uid})
	}), errcode.ErrOrgNotFound)

	var deadMembers int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE org_id = $1`, dead).Scan(&deadMembers))
	assert.Zero(t, deadMembers, "4 条成员写入落点均不得向软删组织写入成员行")

	// 正向对照：存活组织同批落点必须全部成功（FOR SHARE 守卫不得误伤正常写入）
	live := insertOrg(t, "alive", "root.alive", nil)
	require.NoError(t, repo.AddMember(ctx, live, uid, false))
	require.NoError(t, repo.AddMemberWithRole(ctx, live, uid, false, "member", "assigned"))
	require.NoError(t, repo.SetUserOrgs(ctx, uid, []int64{live}, nil))
	require.NoError(t, repo.RunInTx(ctx, func(tx pgx.Tx) error {
		return repo.SetOwnersTx(ctx, tx, live, []int64{uid})
	}))

	var liveMembers int
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT COUNT(*) FROM user_orgs WHERE org_id = $1`, live).Scan(&liveMembers))
	assert.Equal(t, 1, liveMembers, "存活组织应正常写入成员行（守卫不误伤）")
}

//go:build integration

package ticket

// BK-11 ① 回归：Create 的 org_path 快照读必须在事务内以 FOR SHARE 取得，
// 与 Move 的 FOR UPDATE 子树写锁串行化。两个用例：
//  1. 确定性锁窗口：持有 org 行写锁未提交时 Create 必须阻塞，提交后读到新 path；
//  2. 锤击：真实 Move 与并发 Create 交叉，每轮所有工单 org_path == move 后终态 path。
//
// 登记：docs/review/00-implementation-plan.md §9 BK-11。

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// setupBK11 建最小树 root > A > B（B 为被 move 节点）+ 一个建单用户
func setupBK11(t *testing.T) (svc *Service, orgRepo *repository.OrgRepo, aID, bID, actor int64, bCode string) {
	t.Helper()
	svc, _, _, _, _ = setupTicket2a(t)
	orgRepo = repository.NewOrgRepo(testPool)
	ctx := context.Background()
	suffix := fmt.Sprintf("bk11_%d", time.Now().UnixNano()%1e9)
	bCode = "vgbk11_" + suffix

	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ($1, 'A', 1, $2::ltree, 3, 1, 75, false) RETURNING id`,
		"a_"+suffix, "root.a_"+suffix).Scan(&aID))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, org_type, status, sort_order, is_system)
		VALUES ($1, 'B', $2, (SELECT path::text || '.' || $3 FROM organizations WHERE id = $2)::ltree, 3, 1, 75, false)
		RETURNING id`, bCode, aID, bCode).Scan(&bID))

	require.NoError(t, testPool.QueryRow(ctx, fmt.Sprintf(`
		INSERT INTO users (username, password, employee_no, status) VALUES ('u%s', 'hash', 'E%s', 1)
		RETURNING id`, suffix, suffix)).Scan(&actor))
	return
}

// 用例 1：org 行被并发事务写锁持有（未提交）时，Create 必须阻塞在 FOR SHARE 上；
// 写锁方提交后 Create 才完成，且 org_path 是提交后的新 path（而非锁前的旧 path）。
func TestBK11_CreateBlocksBehindConcurrentMove(t *testing.T) {
	svc, _, _, bID, actor, bCode := setupBK11(t)
	ctx := context.Background()
	movedPath := "root." + bCode // move 到 root 后的新 path

	// 模拟 Move 的写锁窗口（等价 Move step 2+5：锁 B 行 + 改 path；B 为叶子无级联），
	// 持锁不提交
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	_, err = tx.Exec(ctx, `SELECT id FROM organizations WHERE id = $1 FOR UPDATE`, bID)
	require.NoError(t, err)
	_, err = tx.Exec(ctx, `UPDATE organizations SET path = $1::ltree, parent_id = 1 WHERE id = $2`, movedPath, bID)
	require.NoError(t, err)

	done := make(chan error, 1)
	var created *model.Ticket
	go func() {
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: "incident", Title: "BK11 阻塞验证", OrgID: bID,
		}, actor)
		created = tk
		done <- err
	}()

	select {
	case err := <-done:
		t.Fatalf("Create 在并发 move 提交前返回（FOR SHARE 未生效），err=%v", err)
	case <-time.After(300 * time.Millisecond):
		// 预期：仍阻塞在 FOR SHARE 上
	}

	require.NoError(t, tx.Commit(ctx))

	select {
	case err := <-done:
		require.NoError(t, err, "move 提交后 Create 应解除阻塞并成功")
	case <-time.After(5 * time.Second):
		t.Fatal("move 提交后 Create 仍未返回")
	}
	require.NotNil(t, created)

	// 以 DB 落库值为准（返回结构体的 OrgPath 是插入前快照，两者必须一致）
	var dbPath string
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT org_path::text FROM tickets WHERE id = $1`, created.ID).Scan(&dbPath))
	assert.Equal(t, movedPath, dbPath, "落库 org_path 必须是 move 提交后的新 path")
}

// 用例 2：真实 Move 与并发 Create 交叉锤击。不变量：每轮结束后，本轮创建的
// 所有工单 org_path == B 的终态 path（要么 Create 串行在 move 后读到新 path，
// 要么先提交被 Move 的 tickets 级联重映射捕获）。
func TestBK11_CreateVsMoveHammer(t *testing.T) {
	svc, orgRepo, aID, bID, actor, _ := setupBK11(t)
	ctx := context.Background()
	rootID := int64(1)

	const rounds = 8
	for i := 0; i < rounds; i++ {
		target := &rootID // 偶数轮 move 到 root
		if i%2 == 1 {
			target = &aID // 奇数轮 move 回 A
		}

		var mu sync.Mutex
		var ticketIDs []int64
		var wg sync.WaitGroup
		for j := 0; j < 3; j++ {
			wg.Add(1)
			go func(n int) {
				defer wg.Done()
				tk, err := svc.Create(ctx, &model.CreateTicketRequest{
					TypeCode: "incident", Title: fmt.Sprintf("BK11 锤击 r%d-c%d", i, n), OrgID: bID,
				}, actor)
				require.NoError(t, err)
				mu.Lock()
				ticketIDs = append(ticketIDs, tk.ID)
				mu.Unlock()
			}(j)
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			require.NoError(t, orgRepo.Move(ctx, bID, target))
		}()
		wg.Wait()

		var finalPath string
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT path::text FROM organizations WHERE id = $1`, bID).Scan(&finalPath))
		for _, id := range ticketIDs {
			var p string
			require.NoError(t, testPool.QueryRow(ctx,
				`SELECT org_path::text FROM tickets WHERE id = $1`, id).Scan(&p))
			assert.Equal(t, finalPath, p, "轮次 %d：工单 %d org_path 未与 move 终态一致", i, id)
		}
	}
}

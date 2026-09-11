//go:build integration

package repository_test

// P1-2 回归：工单关联「反方向判重」并发正确性（迁移 000028）。
// 真 PG（testcontainers）：
//   - 60 轮并发 A→B / B→A（同 relation_type，每轮清场）→ 0 轮出现 2 行（回归历史 59/60 复现）；
//   - 顺序重复（反向 + 同向）→ 409 ErrConflict；
//   - 迁移 up 在「已存在历史重复行」的数据上跑通：解重 + 建规范化唯一索引（事务内验证后回滚）。

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

var relSeq int64

func relUnique(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), atomic.AddInt64(&relSeq, 1))
}

// setupRelationPair 建 1 org + 2 工单（供关联 FK），返回 (aID, bID)，注册清理。
func setupRelationPair(t *testing.T) (int64, int64) {
	t.Helper()
	ctx := context.Background()
	code := relUnique("relorg")
	var orgID, a, b int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, is_virtual, status, sort_order, is_system)
		VALUES ($1, 'rel-org', NULL, $2::ltree, false, 1, 98, false) RETURNING id`, code, code).Scan(&orgID))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO tickets (type_code, title, created_by, org_id, org_path)
		VALUES ('incident', 'rel-A', 0, $1, $2::ltree) RETURNING id`, orgID, code).Scan(&a))
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO tickets (type_code, title, created_by, org_id, org_path)
		VALUES ('incident', 'rel-B', 0, $1, $2::ltree) RETURNING id`, orgID, code).Scan(&b))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM ticket_relations WHERE source_ticket_id IN ($1,$2) OR target_ticket_id IN ($1,$2)`, a, b)
		_, _ = testPool.Exec(ctx, `DELETE FROM tickets WHERE id IN ($1,$2)`, a, b)
		_, _ = testPool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return a, b
}

func relationFilePath(name string) string {
	return filepath.Join("..", "..", "migrations", name)
}

// TestCreateRelation_ReverseDuplicateUnderConcurrency 60 轮锤击：A→B 与 B→A 并发，
// 规范化唯一索引（uq_ticket_relations_normalized）须保证每轮恰好 1 行。
func TestCreateRelation_ReverseDuplicateUnderConcurrency(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()
	a, b := setupRelationPair(t)
	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}

	const rounds = 60
	for i := 0; i < rounds; i++ {
		// 每轮清场（物理删除，下一轮从零开始）
		_, err := testPool.Exec(ctx,
			`DELETE FROM ticket_relations WHERE source_ticket_id IN ($1,$2) OR target_ticket_id IN ($1,$2)`, a, b)
		require.NoError(t, err)

		errs := runConcurrently(2, func(i int) error {
			rel := &model.TicketRelation{RelationType: "related", CreatedBy: 0}
			if i == 0 {
				rel.SourceTicketID, rel.TargetTicketID = a, b
			} else {
				rel.SourceTicketID, rel.TargetTicketID = b, a
			}
			return repo.CreateRelation(ctx, rel)
		})

		var n int
		require.NoError(t, testPool.QueryRow(ctx, `
			SELECT COUNT(*) FROM ticket_relations
			WHERE deleted_at IS NULL AND relation_type = 'related'
			  AND LEAST(source_ticket_id, target_ticket_id) = $1
			  AND GREATEST(source_ticket_id, target_ticket_id) = $2`, lo, hi).Scan(&n))
		require.Equalf(t, 1, n, "轮次 %d：规范化对落库 %d 行（应恰好 1）", i, n)

		ok, conflict := 0, 0
		for _, e := range errs {
			switch {
			case e == nil:
				ok++
			case errors.Is(e, errcode.ErrConflict):
				conflict++
			default:
				t.Fatalf("轮次 %d 非预期错误：%v", i, e)
			}
		}
		require.Equalf(t, 1, ok, "轮次 %d：应恰好 1 个插入成功", i)
		require.Equalf(t, 1, conflict, "轮次 %d：应恰好 1 个 409 冲突", i)
	}
}

// TestCreateRelation_SequentialDuplicateConflict 顺序重复：反向与同向均 409。
func TestCreateRelation_SequentialDuplicateConflict(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()
	a, b := setupRelationPair(t)

	require.NoError(t, repo.CreateRelation(ctx,
		&model.TicketRelation{SourceTicketID: a, TargetTicketID: b, RelationType: "related"}))
	// 反向重复 A→B 与 B→A 视为同一关联
	require.ErrorIs(t, repo.CreateRelation(ctx,
		&model.TicketRelation{SourceTicketID: b, TargetTicketID: a, RelationType: "related"}), errcode.ErrConflict)
	// 同向重复
	require.ErrorIs(t, repo.CreateRelation(ctx,
		&model.TicketRelation{SourceTicketID: a, TargetTicketID: b, RelationType: "related"}), errcode.ErrConflict)
}

// TestMigration000028_DedupHistoricalDuplicates 迁移 up 在含历史重复行数据上跑通。
// 用单事务隔离：drop 索引 → 造 4 行重复（2 同向 + 2 反向）→ 执行迁移 up →
// 断言解重后仅剩 1 行未删 + 索引重建 → 回滚（索引 drop 一并还原，不留副作用）。
func TestMigration000028_DedupHistoricalDuplicates(t *testing.T) {
	ctx := context.Background()
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)

	// 先摘掉规范化唯一索引，才能造出历史重复行
	_, err = tx.Exec(ctx, `DROP INDEX IF EXISTS uq_ticket_relations_normalized`)
	require.NoError(t, err)

	code := relUnique("migorg")
	var orgID, a, b int64
	require.NoError(t, tx.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, is_virtual, status, sort_order, is_system)
		VALUES ($1, 'mig-org', NULL, $2::ltree, false, 1, 99, false) RETURNING id`, code, code).Scan(&orgID))
	require.NoError(t, tx.QueryRow(ctx, `
		INSERT INTO tickets (type_code, title, created_by, org_id, org_path)
		VALUES ('incident', 'mig-A', 0, $1, $2::ltree) RETURNING id`, orgID, code).Scan(&a))
	require.NoError(t, tx.QueryRow(ctx, `
		INSERT INTO tickets (type_code, title, created_by, org_id, org_path)
		VALUES ('incident', 'mig-B', 0, $1, $2::ltree) RETURNING id`, orgID, code).Scan(&b))

	// 4 行重复（无索引约束，可插入）
	_, err = tx.Exec(ctx, `
		INSERT INTO ticket_relations (source_ticket_id, target_ticket_id, relation_type, created_by)
		VALUES ($1,$2,'related',0), ($1,$2,'related',0), ($2,$1,'related',0), ($2,$1,'related',0)`, a, b)
	require.NoError(t, err)

	// 执行迁移 up（多语句 simple protocol；同一事务内）
	sql, err := os.ReadFile(relationFilePath("000028_ticket_relations_normalized.up.sql"))
	require.NoError(t, err)
	_, err = tx.Exec(ctx, string(sql))
	require.NoError(t, err, "迁移 up 在含历史重复行的数据上必须跑通")

	lo, hi := a, b
	if lo > hi {
		lo, hi = hi, lo
	}
	var n int
	require.NoError(t, tx.QueryRow(ctx, `
		SELECT COUNT(*) FROM ticket_relations
		WHERE deleted_at IS NULL AND relation_type = 'related'
		  AND LEAST(source_ticket_id, target_ticket_id) = $1
		  AND GREATEST(source_ticket_id, target_ticket_id) = $2`, lo, hi).Scan(&n))
	require.Equal(t, 1, n, "解重后规范化对应仅剩 1 行未删")

	var idxExists bool
	require.NoError(t, tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM pg_indexes
		WHERE schemaname = current_schema() AND indexname = 'uq_ticket_relations_normalized')`).Scan(&idxExists))
	require.True(t, idxExists, "迁移应重建规范化唯一索引")
}

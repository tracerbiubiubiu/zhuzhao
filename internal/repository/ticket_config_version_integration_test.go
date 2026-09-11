//go:build integration

package repository_test

// P1-3 回归：工单类型/字段/模板三表乐观锁（version CAS，迁移 000027）。
// 真 PG（testcontainers）：验证
//   - version == nil → 保持旧 patch 行为（且版本仍递增）；
//   - 携当前 version → 命中成功并递增；
//   - 陈旧 version → ErrConcurrentModification（409）；
//   - 对象不存在 → 404/90003；
//   - 并发两写携同一 version → 恰好 1 成功、另 1 冲突（消除 lost update）。

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

var cfgSeq int64

// cfgUnique 生成唯一后缀（UnixNano + 原子计数，避免同纳秒碰撞）。
func cfgUnique(prefix string) string {
	return fmt.Sprintf("%s_%d_%d", prefix, time.Now().UnixNano(), atomic.AddInt64(&cfgSeq, 1))
}

// insertCfgType 建独立 ticket_type 并注册清理。
func insertCfgType(t *testing.T, name string) string {
	t.Helper()
	code := cfgUnique("cfgtype")
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO ticket_types (code, name) VALUES ($1, $2)`, code, name)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_type_fields WHERE type_code = $1`, code)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_templates WHERE type_code = $1`, code)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_types WHERE code = $1`, code)
	})
	return code
}

// setupCfgTemplate 建 org + template（依赖一个 type），返回 (typeCode, templateCode)。
func setupCfgTemplate(t *testing.T) (string, string) {
	t.Helper()
	ctx := context.Background()
	typeCode := insertCfgType(t, "cfg-tmpl-type")
	orgCode := cfgUnique("cfgorg")
	var orgID int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO organizations (code, name, parent_id, path, is_virtual, status, sort_order, is_system)
		VALUES ($1, 'cfg-org', NULL, $2::ltree, false, 1, 97, false) RETURNING id`, orgCode, orgCode).Scan(&orgID))
	tmplCode := cfgUnique("cfgtmpl")
	_, err := testPool.Exec(ctx, `
		INSERT INTO ticket_templates (code, name, type_code, org_id, org_path, created_by)
		VALUES ($1, 'cfg-tmpl', $2, $3, $4::ltree, 0)`, tmplCode, typeCode, orgID, orgCode)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM ticket_templates WHERE code = $1`, tmplCode)
		_, _ = testPool.Exec(ctx, `DELETE FROM organizations WHERE id = $1`, orgID)
	})
	return typeCode, tmplCode
}

func mustTypeVersion(t *testing.T, code string) int {
	t.Helper()
	var v int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT version FROM ticket_types WHERE code = $1`, code).Scan(&v))
	return v
}

func mustTemplateVersion(t *testing.T, code string) int {
	t.Helper()
	var v int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT version FROM ticket_templates WHERE code = $1`, code).Scan(&v))
	return v
}

// runConcurrently 并发执行 n 次 fn（同一 close(start) 起跑线对齐），返回各自错误。
func runConcurrently(n int, fn func(i int) error) []error {
	errs := make([]error, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			<-start
			errs[i] = fn(i)
		}(i)
	}
	close(start)
	wg.Wait()
	return errs
}

// assertExactlyOneConflict 断言：errs 中恰好 1 个 nil + 1 个 ErrConcurrentModification。
func assertExactlyOneConflict(t *testing.T, errs []error) {
	t.Helper()
	ok, conflicts := 0, 0
	for _, e := range errs {
		switch {
		case e == nil:
			ok++
		case errors.Is(e, errcode.ErrConcurrentModification):
			conflicts++
		default:
			t.Fatalf("非预期错误：%v", e)
		}
	}
	require.Equal(t, 1, ok, "应恰好 1 个成功")
	require.Equal(t, 1, conflicts, "应恰好 1 个 409 冲突")
}

func TestUpdateTicketTypeOptimisticLock(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()

	t.Run("nil version 保持旧行为且版本递增", func(t *testing.T) {
		code := insertCfgType(t, "cfg-nil")
		name := "n-new"
		got, err := repo.UpdateTicketType(ctx, code, &name, nil, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, "n-new", got.Name)
		assert.Equal(t, 2, mustTypeVersion(t, code), "无 CAS 也应递增版本")
	})

	t.Run("携当前 version 命中并递增", func(t *testing.T) {
		code := insertCfgType(t, "cfg-ok")
		one := 1
		name := "n2"
		_, err := repo.UpdateTicketType(ctx, code, &name, nil, nil, nil, nil, &one)
		require.NoError(t, err)
		assert.Equal(t, 2, mustTypeVersion(t, code))
	})

	t.Run("陈旧 version → ErrConcurrentModification 且不覆盖", func(t *testing.T) {
		code := insertCfgType(t, "cfg-stale")
		one := 1
		nameA := "a"
		_, err := repo.UpdateTicketType(ctx, code, &nameA, nil, nil, nil, nil, &one)
		require.NoError(t, err)
		nameB := "b"
		_, err = repo.UpdateTicketType(ctx, code, &nameB, nil, nil, nil, nil, &one)
		require.ErrorIs(t, err, errcode.ErrConcurrentModification)
		var name string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT name FROM ticket_types WHERE code = $1`, code).Scan(&name))
		assert.Equal(t, "a", name, "陈旧版本不得静默覆盖先写")
	})

	t.Run("类型不存在 → 90003", func(t *testing.T) {
		one := 1
		name := "x"
		_, err := repo.UpdateTicketType(ctx, cfgUnique("no_such_type"), &name, nil, nil, nil, nil, &one)
		require.ErrorIs(t, err, errcode.ErrTicketTypeNotFound)
	})

	t.Run("并发同一 version 恰好 1 成功", func(t *testing.T) {
		code := insertCfgType(t, "cfg-race")
		one := 1
		errs := runConcurrently(2, func(i int) error {
			name := fmt.Sprintf("racer-%d", i)
			_, err := repo.UpdateTicketType(ctx, code, &name, nil, nil, nil, nil, &one)
			return err
		})
		assertExactlyOneConflict(t, errs)
	})
}

func TestUpdateTicketTemplateOptimisticLock(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()

	t.Run("nil version 保持旧行为且版本递增", func(t *testing.T) {
		_, tmpl := setupCfgTemplate(t)
		name := "t-new"
		_, err := repo.UpdateTicketTemplate(ctx, tmpl, &name, nil, nil, nil, nil)
		require.NoError(t, err)
		assert.Equal(t, 2, mustTemplateVersion(t, tmpl))
	})

	t.Run("陈旧 version → ErrConcurrentModification 且不覆盖", func(t *testing.T) {
		_, tmpl := setupCfgTemplate(t)
		one := 1
		nameA := "a"
		_, err := repo.UpdateTicketTemplate(ctx, tmpl, &nameA, nil, nil, nil, &one)
		require.NoError(t, err)
		nameB := "b"
		_, err = repo.UpdateTicketTemplate(ctx, tmpl, &nameB, nil, nil, nil, &one)
		require.ErrorIs(t, err, errcode.ErrConcurrentModification)
		var name string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT name FROM ticket_templates WHERE code = $1`, tmpl).Scan(&name))
		assert.Equal(t, "a", name, "陈旧版本不得静默覆盖先写")
	})

	t.Run("模板不存在 → 404", func(t *testing.T) {
		one := 1
		name := "x"
		_, err := repo.UpdateTicketTemplate(ctx, cfgUnique("no_such_tmpl"), &name, nil, nil, nil, &one)
		require.ErrorIs(t, err, errcode.ErrNotFound)
	})

	t.Run("并发同一 version 恰好 1 成功", func(t *testing.T) {
		_, tmpl := setupCfgTemplate(t)
		one := 1
		errs := runConcurrently(2, func(i int) error {
			name := fmt.Sprintf("tracer-%d", i)
			_, err := repo.UpdateTicketTemplate(ctx, tmpl, &name, nil, nil, nil, &one)
			return err
		})
		assertExactlyOneConflict(t, errs)
	})
}

func TestReplaceTypeFieldsOptimisticLock(t *testing.T) {
	repo := repository.NewTicketRepo(testPool)
	ctx := context.Background()
	fields := []model.TicketTypeField{
		{FieldKey: "f1", FieldLabel: "F1", FieldType: "input", FieldOptions: json.RawMessage("[]"), SortOrder: 10},
	}

	t.Run("nil version 保持旧行为并递增父版本", func(t *testing.T) {
		code := insertCfgType(t, "rf-nil")
		require.NoError(t, repo.ReplaceTypeFields(ctx, code, fields, nil))
		assert.Equal(t, 2, mustTypeVersion(t, code), "无 CAS 也应递增父类型版本")
		var cnt int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM ticket_type_fields WHERE type_code = $1`, code).Scan(&cnt))
		assert.Equal(t, 1, cnt)
	})

	t.Run("陈旧父 version → ErrConcurrentModification", func(t *testing.T) {
		code := insertCfgType(t, "rf-stale")
		one := 1
		require.NoError(t, repo.ReplaceTypeFields(ctx, code, fields, &one))
		err := repo.ReplaceTypeFields(ctx, code, fields, &one)
		require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	})

	t.Run("类型不存在 → 90003", func(t *testing.T) {
		one := 1
		err := repo.ReplaceTypeFields(ctx, cfgUnique("rf_nope"), fields, &one)
		require.ErrorIs(t, err, errcode.ErrTicketTypeNotFound)
	})

	t.Run("并发同一父 version 恰好 1 成功且无重复残留", func(t *testing.T) {
		code := insertCfgType(t, "rf-race")
		one := 1
		errs := runConcurrently(2, func(i int) error {
			f := []model.TicketTypeField{
				{FieldKey: fmt.Sprintf("k%d", i), FieldLabel: "L", FieldType: "input", FieldOptions: json.RawMessage("[]"), SortOrder: 10},
			}
			return repo.ReplaceTypeFields(ctx, code, f, &one)
		})
		assertExactlyOneConflict(t, errs)
		var cnt int
		require.NoError(t, testPool.QueryRow(ctx,
			`SELECT COUNT(*) FROM ticket_type_fields WHERE type_code = $1`, code).Scan(&cnt))
		assert.Equal(t, 1, cnt, "赢家字段集落库，输家不残留")
	})
}

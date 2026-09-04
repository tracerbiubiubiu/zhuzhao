//go:build integration

// 批次 A 平台策略库（authz.md §3.1）集成验证：内置策略的 Filter 谓词在真实 PG
// schema（user_orgs.expires_at 等）上的行为——正负向（成员可见/非成员不可见/
// 过期成员不可见）+ RequireSchema 的 information_schema 语义。
package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
)

// baSuffix 本文件专用随机后缀（repository 测试包无 uniqueSuffix，避免与 service 包耦合）。
func baSuffix() string { return fmt.Sprintf("%d", time.Now().UnixNano()) }

func seedBuiltinUser(t *testing.T, eno string) int64 {
	t.Helper()
	var uid int64
	err := testPool.QueryRow(context.Background(),
		`INSERT INTO users (employee_no, username, password, status) VALUES ($1, $1, 'hash', 1) RETURNING id`, eno).
		Scan(&uid)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, uid) })
	return uid
}

func seedBuiltinOrg(t *testing.T, code string) int64 {
	t.Helper()
	var id int64
	err := testPool.QueryRow(context.Background(), `
		INSERT INTO organizations (code, name, path, is_virtual, status, is_system, sort_order)
		VALUES ($1, $1, $2::ltree, false, 1, false, 1) RETURNING id`, code, code).
		Scan(&id)
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM organizations WHERE id=$1`, id) })
	return id
}

func TestBuiltinOrgMemberFilter(t *testing.T) {
	ctx := context.Background()
	suffix := baSuffix()

	uid := seedBuiltinUser(t, "ba_mem"+suffix)
	other := seedBuiltinUser(t, "ba_oth"+suffix)
	orgOK := seedBuiltinOrg(t, "ba_ok"+suffix)
	orgGone := seedBuiltinOrg(t, "ba_gone"+suffix)
	orgNo := seedBuiltinOrg(t, "ba_no"+suffix)

	// 有效成员关系；过期关系（expires_at 已过）不得放行
	_, err := testPool.Exec(ctx, `INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1,$2,true)`, uid, orgOK)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO user_orgs (user_id, org_id, is_primary, expires_at) VALUES ($1,$2,false,$3)`,
		uid, orgGone, time.Now().Add(-time.Hour))
	require.NoError(t, err)
	t.Cleanup(func() {
		testPool.Exec(ctx, `DELETE FROM user_orgs WHERE user_id IN ($1,$2)`, uid, other)
	})

	// 模块资源表（org_id 关联列）——内置策略谓词拼接到模块列表 SQL 的形态
	tbl := fmt.Sprintf("ba_rows_%s", suffix)
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s (id BIGSERIAL PRIMARY KEY, org_id BIGINT NOT NULL, v TEXT)`, tbl))
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DROP TABLE IF EXISTS `+tbl) })
	for _, org := range []int64{orgOK, orgGone, orgNo} {
		_, err = testPool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (org_id, v) VALUES ($1, 'x')`, tbl), org)
		require.NoError(t, err)
	}

	res := resource.Builtin("ba", resource.OrgMember(tbl))
	f, err := res.GetFilter(ctx, uid, "list")
	require.NoError(t, err)

	var got []int64
	rows, err := testPool.Query(ctx, fmt.Sprintf(`SELECT org_id FROM %s WHERE %s`, tbl, f.Where), f.Args...)
	require.NoError(t, err)
	defer rows.Close()
	for rows.Next() {
		var orgID int64
		require.NoError(t, rows.Scan(&orgID))
		got = append(got, orgID)
	}
	// 正向：有效成员组织可见；负向：过期成员组织、非成员组织不可见
	require.Equal(t, []int64{orgOK}, got)

	// 单条/端点判定：Membership 闭包走同款 EXISTS 语义
	res.Membership = func(ctx context.Context, userID, orgID int64) (bool, error) {
		var ok bool
		err := testPool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM user_orgs
			               WHERE user_id=$1 AND org_id=$2
			                 AND (expires_at IS NULL OR expires_at > NOW()))`,
			userID, orgID).Scan(&ok)
		return ok, err
	}
	ok, err := res.Authorize(ctx, resource.AuthorizeRequest{UserID: uid, Context: map[string]any{"org_id": orgOK}})
	require.NoError(t, err)
	require.True(t, ok, "有效成员应放行")
	ok, err = res.Authorize(ctx, resource.AuthorizeRequest{UserID: uid, Context: map[string]any{"org_id": orgGone}})
	require.NoError(t, err)
	require.False(t, ok, "过期成员应拒绝")
	ok, err = res.Authorize(ctx, resource.AuthorizeRequest{UserID: other, Context: map[string]any{"org_id": orgOK}})
	require.NoError(t, err)
	require.False(t, ok, "非成员应拒绝")
}

func TestBuiltinOwnerOnlyFilter(t *testing.T) {
	ctx := context.Background()
	suffix := baSuffix()
	tbl := fmt.Sprintf("ba_own_%s", suffix)
	_, err := testPool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s (id BIGSERIAL PRIMARY KEY, created_by BIGINT NOT NULL, v TEXT)`, tbl))
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DROP TABLE IF EXISTS `+tbl) })
	uid, other := seedBuiltinUser(t, "ba_ow1"+suffix), seedBuiltinUser(t, "ba_ow2"+suffix)
	_, err = testPool.Exec(ctx, fmt.Sprintf(`INSERT INTO %s (created_by) VALUES ($1),($2)`, tbl), uid, other)
	require.NoError(t, err)

	res := resource.Builtin("ba2", resource.OwnerOnly(tbl))
	f, err := res.GetFilter(ctx, uid, "list")
	require.NoError(t, err)
	var n int64
	require.NoError(t, testPool.QueryRow(ctx,
		fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s`, tbl, f.Where), f.Args...).Scan(&n))
	require.Equal(t, int64(1), n, "属主仅见自己的行")

	// 单条：属主放行、他人拒绝
	ok, _ := res.Authorize(ctx, resource.AuthorizeRequest{UserID: uid, Context: map[string]any{"created_by": uid}})
	require.True(t, ok)
	ok, _ = res.Authorize(ctx, resource.AuthorizeRequest{UserID: other, Context: map[string]any{"created_by": uid}})
	require.False(t, ok)
}

func TestBuiltinRequireSchemaAgainstPG(t *testing.T) {
	ctx := context.Background()
	suffix := baSuffix()
	tbl := fmt.Sprintf("ba_schema_%s", suffix)
	_, err := testPool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s (id BIGSERIAL PRIMARY KEY, org_id BIGINT)`, tbl))
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DROP TABLE IF EXISTS `+tbl) })

	exists := func(table, column string) bool {
		var ok bool
		if err := testPool.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM information_schema.columns
			               WHERE table_name=$1 AND column_name=$2)`, table, column).Scan(&ok); err != nil {
			return false
		}
		return ok
	}
	require.NoError(t, resource.Builtin("ba3", resource.OrgMember(tbl)).RequireSchema(exists))

	noOrgID := fmt.Sprintf("ba_bad_%s", suffix)
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE TABLE %s (id BIGSERIAL PRIMARY KEY)`, noOrgID))
	require.NoError(t, err)
	t.Cleanup(func() { testPool.Exec(ctx, `DROP TABLE IF EXISTS `+noOrgID) })
	require.Error(t, resource.Builtin("ba4", resource.OrgMember(noOrgID)).RequireSchema(exists),
		"缺 org_id 列必须 fail-fast")
}

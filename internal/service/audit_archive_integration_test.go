//go:build integration

// B11② 审计归档集成验证（真 PG）：超期导出 JSONL→同批删行、保留期边界（cutoff）、
// 新鲜行不动、幂等重跑、params 非法 → ErrAbort（P7 不可重试）。
package service_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func seedAuditRow(t *testing.T, path string, createdAt time.Time) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO audit_logs (username, method, path, status_code, duration, created_at)
		 VALUES ('arch_it', 'GET', $1, 200, 1, $2)`, path, createdAt)
	require.NoError(t, err)
}

func seedPolicyEvalRow(t *testing.T, createdAt time.Time) {
	t.Helper()
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO policy_evaluation_logs (actor_id, resource_type, resource_id, action, result, created_at)
		 VALUES (1, 'ticket', 'it', 'read', true, $1)`, createdAt)
	require.NoError(t, err)
}

func countRows(t *testing.T, q string, args ...interface{}) int64 {
	t.Helper()
	var n int64
	require.NoError(t, testPool.QueryRow(context.Background(), q, args...).Scan(&n))
	return n
}

func TestAuditArchiveJob(t *testing.T) {
	ctx := context.Background()
	old := time.Now().Add(-200 * 24 * time.Hour) // 超期（> 180 天默认保留）
	fresh := time.Now()

	t.Run("export then delete, fresh kept, idempotent rerun", func(t *testing.T) {
		// 种子：两张表各 老行×2 + 新行×1（用独立 path 标识本组审计行）
		marker := "arch-" + time.Now().Format("150405.000000000")
		for i := 0; i < 2; i++ {
			seedAuditRow(t, marker, old)
		}
		seedAuditRow(t, marker, fresh)
		seedPolicyEvalRow(t, old)
		seedPolicyEvalRow(t, old)
		seedPolicyEvalRow(t, fresh)

		dir := t.TempDir()
		repo := repository.NewAuditLogRepo(testPool)
		job := service.NewAuditArchiveJob(repo, 180, 2, dir, nil) // batch=2 → 分批路径也被覆盖

		require.NoError(t, job.Handle(ctx, nil))

		// 老行已删、新行保留
		require.EqualValues(t, 1, countRows(t,
			`SELECT COUNT(*) FROM audit_logs WHERE path=$1`, marker))
		require.EqualValues(t, 1, countRows(t,
			`SELECT COUNT(*) FROM policy_evaluation_logs WHERE created_at > NOW() - interval '1 day'`))

		// JSONL：audit_logs 文件含本组老行 2 行；policy_evaluation_logs 老行 2 行
		aLines := readTableArchive(t, dir, "audit_logs")
		got := 0
		for _, l := range aLines {
			if strings.Contains(l, marker) {
				got++
			}
		}
		require.Equal(t, 2, got, "超期审计行应导出 2 行")
		require.Len(t, readTableArchive(t, dir, "policy_evaluation_logs"), 2)

		// 幂等重跑：无超期行（本组），文件不新增、计数不变
		before := len(readTableArchive(t, dir, "audit_logs"))
		require.NoError(t, job.Handle(ctx, nil))
		require.Equal(t, before, len(readTableArchive(t, dir, "audit_logs")))
	})

	t.Run("retention override via params, invalid params abort", func(t *testing.T) {
		dir := t.TempDir()
		repo := repository.NewAuditLogRepo(testPool)
		job := service.NewAuditArchiveJob(repo, 180, 5000, dir, nil)

		// 10 天前的行：默认 180 不动；params retention_days=5 → 归档
		marker := "arch-short-" + time.Now().Format("150405.000000000")
		seedAuditRow(t, marker, time.Now().Add(-10*24*time.Hour))
		require.NoError(t, job.Handle(ctx, nil))
		require.EqualValues(t, 1, countRows(t, `SELECT COUNT(*) FROM audit_logs WHERE path=$1`, marker),
			"默认保留期 180 天：10 天前的行不得归档")

		require.NoError(t, job.Handle(ctx, json.RawMessage(`{"retention_days":5}`)))
		require.EqualValues(t, 0, countRows(t, `SELECT COUNT(*) FROM audit_logs WHERE path=$1`, marker),
			"params 覆盖保留期 5 天后应归档删除")

		// 非法 params → ErrAbort（4xx 不可重试，P7）
		err := job.Handle(ctx, json.RawMessage(`{bad json`))
		require.Error(t, err)
		require.ErrorIs(t, err, jobs.ErrAbort)
	})
}

// readTableArchive 读取目录下某表的全部 JSONL 行（合并多个批次文件）。
func readTableArchive(t *testing.T, dir, table string) []string {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(dir, table+"-*.jsonl"))
	require.NoError(t, err)
	var lines []string
	for _, m := range matches {
		b, err := os.ReadFile(m)
		require.NoError(t, err)
		s := strings.TrimSpace(string(b))
		if s == "" {
			continue
		}
		lines = append(lines, strings.Split(s, "\n")...)
	}
	return lines
}

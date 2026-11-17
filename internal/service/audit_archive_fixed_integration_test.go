//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func TestAuditArchiveFixedAdmissionRepair_RealEffectAndZeroUnapprovedEffect(t *testing.T) {
	ctx := context.Background()
	marker := fmt.Sprint("fixed-background-", time.Now().UnixNano())
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), `DELETE FROM audit_logs WHERE path=$1`, marker)
		require.NoError(t, err)
	})
	seedAuditRow(t, marker, time.Now().Add(-200*24*time.Hour))
	seedAuditRow(t, marker, time.Now())
	dir := t.TempDir()
	repo := repository.NewAuditLogRepo(testPool)
	bare := service.NewAuditArchiveJob(repo, 180, 5000, dir, 395, nil)
	assertNoEffect := func() {
		t.Helper()
		require.EqualValues(t, 2, countRows(t, `SELECT count(*) FROM audit_logs WHERE path=$1`, marker))
		files, err := os.ReadDir(dir)
		require.NoError(t, err)
		require.Empty(t, files)
	}
	for _, params := range []json.RawMessage{nil, json.RawMessage(`{}`), json.RawMessage(`{"retention_days":180}`)} {
		require.ErrorIs(t, bare.Handle(ctx, params), jobs.ErrAbort)
		require.ErrorIs(t, bare.Handle(service.WithTaskPrincipal(ctx, 42, "system"), params), jobs.ErrAbort)
		assertNoEffect()
	}
	cfg := config.ArchiveConfig{CronEnabled: true, CronTick: "30s", RetentionDays: 180, BatchRows: 5000, OutDir: dir, FileRetentionDays: 395}
	fixed, err := service.NewFixedAuditArchiveJob(repo, cfg, nil)
	require.NoError(t, err)
	for _, params := range []json.RawMessage{json.RawMessage(`{"retention_days":5}`), json.RawMessage(`{"approval_id":"forged"}`), json.RawMessage(`{"created_by":"system"}`)} {
		require.ErrorIs(t, fixed.Handle(ctx, params), jobs.ErrAbort)
		assertNoEffect()
	}
	// Fixed system maintenance does not depend on the default-empty human grant list.
	require.NoError(t, fixed.Handle(ctx, nil))
	require.EqualValues(t, 1, countRows(t, `SELECT count(*) FROM audit_logs WHERE path=$1`, marker))
	lines := readTableArchive(t, dir, "audit_logs")
	matched := 0
	for _, line := range lines {
		if strings.Contains(line, marker) {
			matched++
		}
	}
	require.Equal(t, 1, matched, "the old marker is exported exactly once in this successful batch; the fresh marker remains online")
	require.NoError(t, fixed.Handle(ctx, nil))
	require.Len(t, readTableArchive(t, dir, "audit_logs"), len(lines))
}

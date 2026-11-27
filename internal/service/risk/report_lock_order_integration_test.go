//go:build integration

package risk_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestRiskBatchLocksFamiliesInWorkflowOrder(t *testing.T) {
	seedRiskFixture(t)
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	source := seedBasisSource(t)
	svc := newService()
	keys := []string{"lock-a", "lock-b"}
	hash := func(key string) string {
		sum := sha256.Sum256([]byte(source + "\n" + key))
		return hex.EncodeToString(sum[:])
	}
	if hash(keys[0]) < hash(keys[1]) {
		keys[0], keys[1] = keys[1], keys[0]
	}
	input := baseObservation(uniqueSuffix(), "first", "1")
	input.SourceRef = source
	for _, key := range keys {
		input.SourceItemKey = key
		input.EventID = key
		_, err := svc.Report(ctx, model.RiskReportRequest{Observations: []model.FindingObservationV1{input}}, userM)
		require.NoError(t, err)
	}

	var first, second int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT min(id), max(id) FROM risk_families
		WHERE fingerprint_scope=$1`, source).Scan(&first, &second))
	require.Less(t, first, second)
	workflow, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer workflow.Rollback(context.Background())
	_, err = workflow.Exec(ctx, `SELECT id FROM risk_families WHERE id=$1 FOR UPDATE`, first)
	require.NoError(t, err)

	report, err := testPool.Begin(ctx)
	require.NoError(t, err)
	reportPID := report.Conn().PgConn().PID()
	reportDone := make(chan error, 1)
	go func() {
		defer report.Rollback(context.Background())
		reportDone <- repository.LockObservedRiskItemsTx(ctx, report, source, 1, []string{hash(keys[0]), hash(keys[1])})
	}()

	require.Eventually(t, func() bool {
		var waiting bool
		err := testPool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity
			WHERE pid=$1 AND wait_event_type='Lock')`, reportPID).Scan(&waiting)
		return err == nil && waiting
	}, 3*time.Second, 20*time.Millisecond)
	// Existing workflow locking visits the next family by ascending ID.
	// The report must wait at the first family without blocking that step.
	_, err = workflow.Exec(ctx, `SELECT id FROM risk_families WHERE id=$1 FOR UPDATE NOWAIT`, second)
	require.NoError(t, err)
	require.NoError(t, workflow.Commit(ctx))
	require.NoError(t, <-reportDone)
}

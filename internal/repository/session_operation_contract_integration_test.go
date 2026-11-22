//go:build integration

package repository_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestC05ProjectionConfirmationRequiresCurrentFacts(t *testing.T) {
	for _, scenario := range []string{"current", "stale-epoch", "expired-lease", "wrong-state", "wrong-fence"} {
		t.Run(scenario, func(t *testing.T) {
			ctx := context.Background()
			repo, op, intent := projectionFixture(t, ctx)
			fence, ok, err := repo.AcquireIntentLease(ctx, intent.ID, "fixture-lease", time.Minute)
			require.NoError(t, err)
			require.True(t, ok)
			switch scenario {
			case "stale-epoch":
				_, err = testPool.Exec(ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, op.TargetUserID)
			case "expired-lease":
				_, err = testPool.Exec(ctx, `UPDATE session_projection_intents SET lease_expires_at=clock_timestamp()-interval '1 second' WHERE id=$1`, intent.ID)
			case "wrong-state":
				_, err = testPool.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, op.TargetUserID)
			case "wrong-fence":
				fence++
			}
			require.NoError(t, err)
			confirmed, err := repo.ConfirmIntent(ctx, intent.ID, "fixture-lease", fence, intent.TargetEpoch)
			require.NoError(t, err)
			var projected int64
			var status string
			require.NoError(t, testPool.QueryRow(ctx, `
				SELECT u.session_projection_epoch, i.status
				FROM users u JOIN session_projection_intents i ON i.user_id=u.id WHERE i.id=$1`, intent.ID).
				Scan(&projected, &status))
			if scenario == "current" {
				require.True(t, confirmed)
				require.Equal(t, intent.TargetEpoch, projected)
				require.Equal(t, "confirmed", status)
				return
			}
			require.False(t, confirmed)
			require.Zero(t, projected)
			require.Equal(t, "pending", status)
		})
	}
}

func TestC05ProjectionRepairPreservesOriginalOperation(t *testing.T) {
	ctx := context.Background()
	repo, op, intent := projectionFixture(t, ctx)
	_, err := testPool.Exec(ctx, `DELETE FROM session_projection_intents WHERE id=$1`, intent.ID)
	require.NoError(t, err)
	for range 2 {
		tx, err := testPool.Begin(ctx)
		require.NoError(t, err)
		defer tx.Rollback(ctx)
		require.NoError(t, repo.EnsureIntentTx(ctx, tx, op, intent))
		require.NoError(t, tx.Commit(ctx))
	}
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM session_projection_intents WHERE operation_id=$1 AND user_id=$2 AND target_epoch=1`, op.OperationID, op.TargetUserID).Scan(&count))
	require.Equal(t, 1, count)

	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	conflicting := *op
	conflicting.Command = "user.status"
	require.ErrorIs(t, repo.EnsureIntentTx(ctx, tx, &conflicting, intent), errcode.ErrConflict)
}

func projectionFixture(t *testing.T, ctx context.Context) (*repository.SessionOperationRepo, *repository.SessionOperation, *repository.SessionProjectionIntent) {
	t.Helper()
	tx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer tx.Rollback(ctx)
	var userID int64
	require.NoError(t, tx.QueryRow(ctx, `INSERT INTO users(username,password) VALUES($1,'synthetic') RETURNING id`,
		fmt.Sprintf("c05_fixture_%d", time.Now().UnixNano())).Scan(&userID))
	repo := repository.NewSessionOperationRepo(testPool)
	epoch := int64(1)
	op := &repository.SessionOperation{
		OperationID: fmt.Sprintf("bootstrap:%d", userID), Namespace: "bootstrap", Command: "user.create",
		TargetUserID: userID, ActorUserID: userID, ExpectedEpoch: epoch, Status: "completed", ResultEpoch: &epoch,
	}
	require.NoError(t, repo.InsertOperationTx(ctx, tx, op))
	intent := &repository.SessionProjectionIntent{OperationID: op.OperationID, UserID: userID, TargetEpoch: epoch, DesiredActive: true, Reason: "bootstrap-backfill"}
	require.NoError(t, repo.InsertIntentTx(ctx, tx, intent))
	require.NoError(t, tx.QueryRow(ctx, `SELECT id FROM session_projection_intents WHERE operation_id=$1`, op.OperationID).Scan(&intent.ID))
	require.NoError(t, tx.Commit(ctx))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM session_projection_intents WHERE user_id=$1`, userID)
		_, _ = testPool.Exec(ctx, `DELETE FROM user_session_operations WHERE target_user_id=$1`, userID)
		_, _ = testPool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID)
	})
	return repo, op, intent
}

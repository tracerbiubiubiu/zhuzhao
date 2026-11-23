//go:build integration

package riskdispatch

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestPersistedPlanBindsActorAndOperationBeforeGroups(t *testing.T) {
	db := dispatchTestDB
	ctx := context.Background()

	now := time.Date(2026, 11, 22, 8, 0, 0, 0, time.UTC)
	target := Target{OrganizationID: 30, TypeCode: "synthetic_remediation", DefinitionVersion: 1, DefinitionHash: "published"}
	binding := BuildPlan(target, []Candidate{{
		RiskID: 77, Revision: 2, FindingKey: "finding-001", AssetKey: "asset-1",
		OwnerID: 21, ManagementOrgID: 20, ApprovedScope: "scope-1", Priority: 2, Deadline: now.Add(8 * time.Hour),
	}})

	tx, err := db.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = tx.Rollback(ctx) })
	first, err := SavePlanTx(ctx, tx, 17, target, binding, now)
	require.NoError(t, err)
	second, err := SavePlanTx(ctx, tx, 17, target, binding, now)
	require.NoError(t, err)
	require.NotEqual(t, first.ID, second.ID)
	require.Equal(t, first.Hash, second.Hash)

	stored, err := ReadPlan(ctx, tx, 17, first.ID)
	require.NoError(t, err)
	require.Equal(t, first, stored)
	require.Equal(t, now.Add(15*time.Minute), stored.ExpiresAt)

	_, err = ReadPlan(ctx, tx, 18, first.ID)
	require.ErrorIs(t, err, errcode.ErrNotFound)

	_, err = ReadPlan(ctx, tx, 17, uuid.New())
	require.ErrorIs(t, err, errcode.ErrNotFound)

	err = BindOperationTx(ctx, tx, 17, "synthetic-op-1", first.ID, first.Hash)
	require.NoError(t, err)
	err = BindOperationTx(ctx, tx, 17, "synthetic-op-1", first.ID, first.Hash)
	require.NoError(t, err)
	err = BindOperationTx(ctx, tx, 17, "synthetic-op-1", second.ID, second.Hash)
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	require.NoError(t, tx.Commit(ctx))

	stored, err = ReadPlan(ctx, db, 17, first.ID)
	require.NoError(t, err)
	require.Equal(t, first, stored)
}

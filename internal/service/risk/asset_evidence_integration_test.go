//go:build integration

package risk_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/riskasset"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

type lookupDirectory func(context.Context, riskasset.LookupRequest) (riskasset.LookupResult, error)

func (f lookupDirectory) Lookup(ctx context.Context, req riskasset.LookupRequest) (riskasset.LookupResult, error) {
	return f(ctx, req)
}

func TestAssetEvidenceIsScopedAndCannotConfirmOwner(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	op := uuid.NewString()
	result, err := svc.Create(ctx, model.RiskCreateRequest{
		OperationID: op, Title: "asset-evidence", Description: "synthetic check", OrgID: orgGID,
		ExpectedRevision: "0",
	}, userM)
	require.NoError(t, err)
	id := itemIDByOperation(t, op)
	_, err = testPool.Exec(ctx, `UPDATE risk_items SET asset_provider='synthetic', asset_external_id='asset-a' WHERE id=$1`, id)
	require.NoError(t, err)

	calls := 0
	directory := lookupDirectory(func(_ context.Context, req riskasset.LookupRequest) (riskasset.LookupResult, error) {
		calls++
		require.Equal(t, riskasset.Ref{Provider: "synthetic", ExternalID: "asset-a"}, req.AssetRef)
		require.NotEmpty(t, req.RequestID)
		return riskasset.LookupResult{Status: riskasset.StatusStale, Evidence: &riskasset.Evidence{
			AssetRef: req.AssetRef, ObservedAt: time.Now().Add(-time.Hour), OwnerExternalRef: "external-unconfirmed",
		}}, nil
	})
	_, err = svc.AssetEvidence(ctx, id, 0, directory)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)
	require.Zero(t, calls)

	evidence, err := svc.AssetEvidence(ctx, id, userM, directory)
	require.NoError(t, err)
	require.Equal(t, "stale", evidence.Status)
	require.Equal(t, "external-unconfirmed", evidence.Evidence.OwnerExternalRef)
	require.Equal(t, 1, calls)
	detail, err := svc.Detail(ctx, id, userM)
	require.NoError(t, err)
	require.Nil(t, detail.View.OwnerUserID)
	require.Equal(t, result.Risk.Revision, detail.View.Risk.Revision)

	timeout, err := svc.AssetEvidence(ctx, id, userM, lookupDirectory(func(context.Context, riskasset.LookupRequest) (riskasset.LookupResult, error) {
		return riskasset.LookupResult{Status: riskasset.StatusTimeout}, errors.New("unrelated upstream detail")
	}))
	require.NoError(t, err)
	require.Equal(t, "timeout", timeout.Status)
	require.Nil(t, timeout.Evidence)
}

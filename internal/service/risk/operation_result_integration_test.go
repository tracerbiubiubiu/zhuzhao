//go:build integration

package risk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

func TestRiskOperationResultReturnsOriginalRevisionToAuthorizedActor(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	operationID := "original-risk-operation-" + uniqueSuffix()
	created := mustCreate(t, svc, operationID, "synthetic operation readback", orgGID, nil)

	receipt, err := svc.OperationResult(ctx, operationID, userM)
	require.NoError(t, err)
	require.Equal(t, operationID, receipt.OperationID)
	require.Equal(t, created.Risk.ID, receipt.RiskID)
	require.Equal(t, created.Risk.Revision, receipt.Revision)

	_, err = svc.OperationResult(ctx, operationID, userV)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)
}

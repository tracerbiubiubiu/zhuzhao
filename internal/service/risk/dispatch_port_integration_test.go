//go:build integration

package risk_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskdispatch"
)

func TestDispatchRiskAuthoritySeparatesOwnerSingleAndManagerBulk(t *testing.T) {
	seedRiskFixture(t)
	ctx := context.Background()
	svc := newService()
	created := mustCreate(t, svc, "dispatch-guard-"+uniqueSuffix(), "synthetic dispatch owner", orgGID, &userU)
	ids := []int64{created.Risk.ID}
	target := riskdispatch.Target{OrganizationID: orgDID, TypeCode: "synthetic"}

	require.NoError(t, svc.AuthorizeDispatch(ctx, testPool, userM, riskdispatch.ScopeBulk, ids, target))
	require.NoError(t, svc.AuthorizeDispatch(ctx, testPool, userU, riskdispatch.ScopeSingle, ids, target))
	require.ErrorIs(t, svc.AuthorizeDispatch(ctx, testPool, userU, riskdispatch.ScopeBulk, ids, target), errcode.ErrForbidden)
	require.ErrorIs(t, svc.AuthorizeDispatch(ctx, testPool, userV, riskdispatch.ScopeSingle, ids, target), errcode.ErrForbidden)
}

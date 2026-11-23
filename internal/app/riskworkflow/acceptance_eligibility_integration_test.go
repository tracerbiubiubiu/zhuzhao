//go:build integration

package riskworkflow

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
)

func TestWorkflowAcceptanceNeedsCurrentlyEligibleIndependentApprover(t *testing.T) {
	for _, tc := range []struct {
		name string
		sql  string
		args func(*writeFixture) []any
	}{
		{
			name: "disabled account",
			sql:  `UPDATE users SET status=0 WHERE id=$1`,
			args: func(f *writeFixture) []any { return []any{f.actor} },
		},
		{
			name: "lost primary scope",
			sql:  `UPDATE user_orgs SET org_member_role='member' WHERE user_id=$1 AND org_id=$2`,
			args: func(f *writeFixture) []any { return []any{f.actor, f.orgA} },
		},
		{
			name: "lost decision route",
			sql:  `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2 AND v2='POST'`,
			args: func(f *writeFixture) []any { return []any{"role::" + f.role, acceptDecisionPath} },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newAcceptanceFixture(t)
			c := f.acceptanceRequest(t, f.riskA)
			var before string
			got, err := f.admittedWrite(t, c.ActorID, acceptRequestPath, func() {
				_, err := workflowPool.Exec(f.ctx, tc.sql, tc.args(f)...)
				require.NoError(t, err)
				before = f.acceptanceSnapshot(t)
			}, func(ctx context.Context) (riskremediation.Result, error) {
				return f.service.RequestAcceptance(ctx, c)
			})
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, before, f.acceptanceSnapshot(t))
		})
	}
}

func TestWorkflowAcceptanceNeedsOneIndependentApproverForBothPinnedDecisions(t *testing.T) {
	// The creator can approve. The assignee can reject and is the applicant.
	// Both are managers with the decision route, and both edges exist.
	f := prepareAcceptanceFixture(t, newWriteFixtureWithRoles(t, "assignee", "creator", "assignee"), false)
	c := f.acceptanceRequest(t, f.riskA)
	before := f.acceptanceSnapshot(t)
	got, err := f.requestAcceptance(t, c)
	require.Error(t, err)
	require.Empty(t, got)
	require.Equal(t, before, f.acceptanceSnapshot(t))
}

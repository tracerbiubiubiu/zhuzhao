//go:build integration

package risk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

func TestRiskAssessSupplementFillsMissingIdentityOnOriginalRisk(t *testing.T) {
	seedRiskFixture(t)
	ctx := t.Context()
	svc := newService()
	suffix := uniqueSuffix()
	obs := baseObservation(suffix, "supplement", "1")
	obs.SourceRef = seedBasisSource(t)
	obs.SourceItemKey = "supplement-" + suffix
	obs.AssetRef = nil
	obs.EvidenceRefs = []model.RiskEvidenceRef{{Provider: "synthetic-feed", ExternalID: "source-assertion"}}
	report, err := svc.ReportFromSource(ctx, model.RiskReportRequest{
		Observations: []model.FindingObservationV1{obs},
	}, obs.SourceRef)
	require.NoError(t, err)
	ref := report.Items[0].RiskRef
	require.NotNil(t, ref)
	originalMirror := mirrorHashOf(t, obs.SourceRef, obs.SourceItemKey)

	latest := obs
	latest.EventID = "supplement-new-evidence-" + suffix
	latest.SourceSequence = "2"
	latest.EvidenceRefs = []model.RiskEvidenceRef{{Provider: "later-feed-evidence", ExternalID: "source-assertion"}}
	advance, err := svc.ReportFromSource(ctx, model.RiskReportRequest{
		Observations: []model.FindingObservationV1{latest},
	}, obs.SourceRef)
	require.NoError(t, err)
	require.Equal(t, "accepted", advance.Items[0].Result)
	require.NotNil(t, advance.Items[0].RiskRef)
	require.Equal(t, ref.ID, advance.Items[0].RiskRef.ID)
	latestMirror := mirrorHashOf(t, obs.SourceRef, obs.SourceItemKey)
	require.NotEqual(t, originalMirror, latestMirror)

	assigned, err := svc.Assign(ctx, model.RiskAssignRequest{
		OperationID: "assign-supplement-" + suffix, ID: ref.ID, ExpectedRevision: "1",
		OwnerUserID: userU, Reason: "合成责任已核",
	}, userM)
	require.NoError(t, err)
	expiry := time.Now().UTC().Add(time.Hour)
	expectedAsset := model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "confirmed-asset"}
	makeRequest := func(operation, revision string, expiry time.Time, findingKey, evidenceProvider string) model.RiskAssessRequest {
		t.Helper()
		input := fmt.Sprintf(`{"id":"%d","operation_id":%q,"expected_revision":%q,"level":"medium","treatment":"mitigate",
	"assessment_note":"查证独立合成材料","dispatch_basis_valid_until":%q,
	"identity_supplement":{"finding_key":%q,"asset_ref":{"provider":"mock-cmdb","external_id":"confirmed-asset"},
	"evidence_refs":[{"provider":%q,"external_id":"source-assertion"}]}}`,
			ref.ID, operation, revision, expiry.Format(time.RFC3339Nano), findingKey, evidenceProvider)
		var req model.RiskAssessRequest
		require.NoError(t, json.Unmarshal([]byte(input), &req))
		return req
	}
	op := "assess-supplement-" + suffix
	req := makeRequest(op, "2", expiry, obs.FindingKey, "manual-evidence")
	_, err = svc.Assess(ctx, req, userU)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	_, err = svc.Assess(ctx, req, userN)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)
	_, err = svc.Assess(ctx, makeRequest(op, "2", expiry, obs.FindingKey, "synthetic-feed"), userM)
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	_, err = svc.Assess(ctx, makeRequest(op, "2", expiry, obs.FindingKey, "later-feed-evidence"), userM)
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	_, err = svc.Assess(ctx, makeRequest(op, "2", time.Now().UTC().Add(-time.Minute), obs.FindingKey, "manual-evidence"), userM)
	require.ErrorIs(t, err, errcode.ErrInvalidParams)
	_, err = svc.Assess(ctx, makeRequest(op, "2", expiry, "other-finding", "manual-evidence"), userM)
	require.ErrorIs(t, err, risk.ErrRiskInvalidState)
	_, err = svc.Assess(ctx, makeRequest(op, "1", expiry, obs.FindingKey, "manual-evidence"), userM)
	require.ErrorIs(t, err, errcode.ErrConcurrentModification)
	before, err := svc.Detail(ctx, ref.ID, userM)
	require.NoError(t, err)
	require.Equal(t, assigned.Risk, before.View.Risk)
	require.Nil(t, before.View.AssetRef)

	updated, err := svc.Assess(ctx, req, userM)
	require.NoError(t, err)
	require.Equal(t, ref.ID, updated.Risk.ID)
	require.Equal(t, ref.HumanKey, updated.Risk.HumanKey)
	require.Equal(t, int64(3), updated.Risk.Revision)
	detail, err := svc.Detail(ctx, ref.ID, userM)
	require.NoError(t, err)
	require.Equal(t, obs.FindingKey, detail.View.FindingKey)
	require.Equal(t, &expectedAsset, detail.View.AssetRef)
	require.Equal(t, &userU, detail.View.OwnerUserID)
	require.Equal(t, latestMirror, mirrorHashOf(t, obs.SourceRef, obs.SourceItemKey))

	recovered, err := svc.OperationResult(ctx, op, userM)
	require.NoError(t, err)
	require.Equal(t, updated.Risk.Revision, recovered.Revision)
	replay, err := svc.Assess(ctx, req, userM)
	require.NoError(t, err)
	require.Equal(t, updated.Risk.ID, replay.Risk.ID)
	require.Equal(t, updated.Risk.Revision, replay.Risk.Revision)
	require.True(t, replay.Receipt.Replayed)
	_, err = svc.Assess(ctx, makeRequest(op, "2", expiry, obs.FindingKey, "another-evidence"), userM)
	require.ErrorIs(t, err, risk.ErrRiskOperationConflict)
	history, err := svc.History(ctx, ref.ID, 1, 50, userM)
	require.NoError(t, err)
	var assessments int
	for _, event := range history.Items {
		if event.EventKind != "assessed" {
			continue
		}
		assessments++
		require.Equal(t, op, event.OperationID)
		require.Equal(t, userM, event.ActorID)
		require.JSONEq(t, fmt.Sprintf(
			`{"level":"medium","treatment":"mitigate","deadline":null,"note":"查证独立合成材料","dispatch_basis_valid_until":%q,
	"identity_supplement":{"finding_key":%q,"asset_ref":{"provider":"mock-cmdb","external_id":"confirmed-asset"},
	"evidence_refs":[{"provider":"manual-evidence","external_id":"source-assertion"}]}}`,
			expiry.Format(time.RFC3339Nano), obs.FindingKey), string(event.Payload))
	}
	require.Equal(t, 1, assessments)

	_, err = testPool.Exec(ctx, `UPDATE user_orgs SET org_member_role='member'
		WHERE user_id=$1 AND org_id=$2`, userM, orgGID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, restoreErr := testPool.Exec(context.Background(), `UPDATE user_orgs SET org_member_role='admin'
			WHERE user_id=$1 AND org_id=$2`, userM, orgGID)
		require.NoError(t, restoreErr)
	})
	_, err = svc.Assess(ctx, req, userM)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	_, err = svc.OperationResult(ctx, op, userM)
	require.ErrorIs(t, err, risk.ErrRiskNotFound)
}

func TestRiskAssessSupplementDispatchRequiresCurrentSource(t *testing.T) {
	seedRiskFixture(t)
	ctx := t.Context()
	svc := newService()
	source := seedBasisSource(t)
	now := time.Now().UTC()
	expiry := now.Add(time.Hour)
	asset := model.RiskAssetRef{Provider: "mock-cmdb", ExternalID: "confirmed-asset"}

	for _, tc := range []struct {
		name     string
		inactive func(*model.FindingObservationV1)
	}{
		{"expired", func(o *model.FindingObservationV1) {
			expired := now.Add(-time.Minute)
			o.ValidUntil = &expired
		}},
		{"withdrawn", func(o *model.FindingObservationV1) {
			o.ObservationKind = "withdrawn"
		}},
		{"identity_mismatch", func(o *model.FindingObservationV1) {
			o.AssetRef = &model.RiskAssetRef{Provider: asset.Provider, ExternalID: "different-asset"}
		}},
		{"changed_literal_identity", func(o *model.FindingObservationV1) {
			o.AssetRef = &model.RiskAssetRef{Provider: asset.Provider, ExternalID: asset.ExternalID + " "}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			suffix := uniqueSuffix()
			first := baseObservation(suffix, "initial", "1")
			first.SourceRef = source
			first.SourceItemKey = "basis-supplement-" + suffix
			first.AssetRef = nil
			first.ObservedAt = now.Add(-2 * time.Minute)
			first.ValidUntil = &expiry
			report := func(obs model.FindingObservationV1) *model.RiskRef {
				t.Helper()
				r, err := svc.ReportFromSource(ctx, model.RiskReportRequest{
					Observations: []model.FindingObservationV1{obs},
				}, source)
				require.NoError(t, err)
				require.Equal(t, "accepted", r.Items[0].Result)
				require.NotNil(t, r.Items[0].RiskRef)
				return r.Items[0].RiskRef
			}
			ref := report(first)
			identity := &model.RiskIdentitySupplement{
				FindingKey: first.FindingKey, AssetRef: asset,
				EvidenceRefs: []model.RiskEvidenceRef{{Provider: "manual-evidence", ExternalID: "local-review"}},
			}
			assess := func(ref *model.RiskRef, operation string, evidence *model.RiskIdentitySupplement) *model.RiskRef {
				t.Helper()
				req := model.RiskAssessRequest{
					ID: ref.ID, ExpectedRevision: fmt.Sprint(ref.Revision), OperationID: operation + "-" + suffix,
					Level: "medium", Treatment: "mitigate", AssessmentNote: "合成证据核实", IdentitySupplement: evidence,
				}
				if evidence != nil {
					req.DispatchBasisValidUntil = &expiry
				}
				res, err := svc.Assess(ctx, req, userM)
				require.NoError(t, err)
				return &res.Risk
			}
			ref = assess(ref, "plain", nil)
			require.Empty(t, currentBasis(t, svc, ref.ID))
			ref = assess(ref, "supplement", identity)
			basis := currentBasis(t, svc, ref.ID)
			require.NotEmpty(t, basis)
			require.Equal(t, basis, lockBasis(t, svc, ref.ID))

			ref = assess(ref, "revoke", nil)
			require.Empty(t, lockBasis(t, svc, ref.ID))
			ref = assess(ref, "renew", identity)
			renewedBasis := lockBasis(t, svc, ref.ID)
			require.NotEmpty(t, renewedBasis)
			require.NotEqual(t, basis, renewedBasis)

			inactive := first
			inactive.EventID = "inactive-" + suffix
			inactive.SourceSequence = "2"
			inactive.ObservedAt = first.ObservedAt.Add(10 * time.Second)
			tc.inactive(&inactive)
			require.Equal(t, ref.ID, report(inactive).ID)
			require.Empty(t, currentBasis(t, svc, ref.ID))
			require.Empty(t, lockBasis(t, svc, ref.ID))
		})
	}
}

func TestRiskAssessSupplementFillsLegacyBlankIdentity(t *testing.T) {
	seedRiskFixture(t)
	ctx := t.Context()
	svc := newService()
	suffix := uniqueSuffix()
	obs := baseObservation(suffix, "legacy-blank", "1")
	obs.SourceRef = seedBasisSource(t)
	obs.SourceItemKey = "legacy-blank-" + suffix
	// v102 accepts an observation with blank identity slots and preserves their
	// text in the risk and mirror. The existing non-blank external ID is valid.
	obs.FindingKey = "   "
	obs.AssetRef = &model.RiskAssetRef{Provider: "  ", ExternalID: "retained-asset"}
	report, err := svc.ReportFromSource(ctx, model.RiskReportRequest{
		Observations: []model.FindingObservationV1{obs},
	}, obs.SourceRef)
	require.NoError(t, err)
	riskRef := report.Items[0].RiskRef
	require.NotNil(t, riskRef)
	originalMirror := mirrorHashOf(t, obs.SourceRef, obs.SourceItemKey)
	before, err := svc.Detail(ctx, riskRef.ID, userM)
	require.NoError(t, err)
	require.Equal(t, obs.FindingKey, before.View.FindingKey)
	require.Equal(t, obs.AssetRef, before.View.AssetRef)
	require.Empty(t, currentBasis(t, svc, riskRef.ID))

	expiry := time.Now().UTC().Add(time.Hour)
	req := model.RiskAssessRequest{
		ID: riskRef.ID, ExpectedRevision: fmt.Sprint(riskRef.Revision), OperationID: "legacy-blank-" + suffix,
		Level: "medium", Treatment: "mitigate", AssessmentNote: "Synthetic independent identity check",
		DispatchBasisValidUntil: &expiry,
		IdentitySupplement: &model.RiskIdentitySupplement{
			FindingKey: "verified-finding",
			AssetRef: model.RiskAssetRef{
				Provider: "verified-provider", ExternalID: "attempted-replacement",
			},
			EvidenceRefs: []model.RiskEvidenceRef{{Provider: "synthetic-review", ExternalID: "independent-evidence"}},
		},
	}
	_, err = svc.Assess(ctx, req, userM)
	require.ErrorIs(t, err, risk.ErrRiskInvalidState, "cannot replace a non-blank source identity")
	unchanged, err := svc.Detail(ctx, riskRef.ID, userM)
	require.NoError(t, err)
	require.Equal(t, before, unchanged)

	req.IdentitySupplement.AssetRef.ExternalID = obs.AssetRef.ExternalID
	assessed, err := svc.Assess(ctx, req, userM)
	require.NoError(t, err)
	require.Equal(t, riskRef.ID, assessed.Risk.ID)
	require.Equal(t, riskRef.Revision+1, assessed.Risk.Revision)
	verified, err := svc.Detail(ctx, riskRef.ID, userM)
	require.NoError(t, err)
	require.Equal(t, req.IdentitySupplement.FindingKey, verified.View.FindingKey)
	require.Equal(t, &req.IdentitySupplement.AssetRef, verified.View.AssetRef)
	require.Equal(t, originalMirror, mirrorHashOf(t, obs.SourceRef, obs.SourceItemKey))
	require.Contains(t, currentBasis(t, svc, riskRef.ID), "src2:")
}

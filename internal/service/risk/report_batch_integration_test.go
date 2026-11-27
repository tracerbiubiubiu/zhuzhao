//go:build integration

package risk_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
)

func seedReportBatch(t *testing.T, batchID string) (string, model.RiskReportRequest) {
	t.Helper()
	seedRiskFixture(t)
	source := "mock-durable-scan-" + uniqueSuffix()
	_, err := testPool.Exec(t.Context(), `
INSERT INTO risk_sources(source_ref, display_name, fingerprint_scope, ordered, mapping_ref, allowed_org_ids, default_org_id, enabled)
VALUES ($1,'Durable synthetic scan',$1,true,$2,ARRAY[$3::bigint],$3,true)`, source, sourceMapping, orgGID)
	require.NoError(t, err)
	observation := baseObservation(uniqueSuffix(), "durable", "10")
	observation.SourceRef = source
	return source, model.RiskReportRequest{BatchID: batchID, Observations: []model.FindingObservationV1{observation}}
}

func TestRiskFeedExplicitBatchResultIsFrozenAndBoundToInput(t *testing.T) {
	source, request := seedReportBatch(t, "synthetic-batch")
	svc := newService()
	first, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.NotEmpty(t, first.BatchReceipt)
	require.Len(t, first.Items, 1)
	item := first.Items[0]
	require.Equal(t, "accepted", item.Result)
	require.NotNil(t, item.RiskRef)

	_, err = svc.Assess(t.Context(), model.RiskAssessRequest{
		OperationID: request.Observations[0].EventID, ID: item.RiskRef.ID, ExpectedRevision: "1",
		Level: "high", Treatment: "mitigate", AssessmentNote: "Synthetic review",
	}, userM)
	require.NoError(t, err)

	t.Run("exact input after change", func(t *testing.T) {
		replay, err := newService().ReportFromSource(t.Context(), request, source)
		require.NoError(t, err)
		require.Equal(t, first, replay)
	})
	t.Run("same batch ID different payload", func(t *testing.T) {
		changed := request.Observations[0]
		changed.ConclusionRef = "another synthetic conclusion"
		_, err := newService().ReportFromSource(t.Context(), model.RiskReportRequest{
			BatchID: request.BatchID, Observations: []model.FindingObservationV1{changed},
		}, source)
		require.ErrorIs(t, err, risk.ErrRiskOperationConflict)
	})
	t.Run("source scope moves", func(t *testing.T) {
		_, err := testPool.Exec(t.Context(), `
UPDATE risk_sources SET default_org_id=$2, allowed_org_ids=ARRAY[$2::bigint] WHERE source_ref=$1`, source, orgDID)
		require.NoError(t, err)
		_, err = newService().ReportFromSource(t.Context(), request, source)
		require.ErrorIs(t, err, risk.ErrRiskSourceScope)
	})
}

func TestRiskFeedUnkeyedBatchKeepsPerEventReplay(t *testing.T) {
	source, request := seedReportBatch(t, "")
	held := request.Observations[0]
	held.EventID = "held-" + held.EventID
	held.SourceSequence = ""
	request.Observations = append(request.Observations, held)
	first, err := newService().ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, "accepted", first.Items[0].Result)
	require.Equal(t, "held", first.Items[1].Result)
	require.Equal(t, "missing_sequence", first.Items[1].SafeReason)

	replayRequest := model.RiskReportRequest{Observations: append([]model.FindingObservationV1(nil), request.Observations...)}
	for i := range replayRequest.Observations {
		replayRequest.Observations[i].ObservedAt = request.Observations[i].ObservedAt.In(time.FixedZone("synthetic", 3600))
		replayRequest.Observations[i].ObservationKind = "observed"
	}
	replay, err := newService().ReportFromSource(t.Context(), replayRequest, source)
	require.NoError(t, err)
	require.Equal(t, first.BatchReceipt, replay.BatchReceipt)
	for i, item := range replay.Items {
		require.Equal(t, "duplicate", item.Result)
		require.Equal(t, first.Items[i].IngestID, item.IngestID)
		require.Equal(t, first.Items[i].SafeReason, item.SafeReason)
		require.Equal(t, first.Items[i].Processing, item.Processing)
	}
}

func TestRiskFeedInterruptedApplicationResumesReservedIdentity(t *testing.T) {
	for _, method := range []string{"redelivery", "consumer"} {
		t.Run(method, func(t *testing.T) {
			source, request := seedReportBatch(t, "original")
			observation := request.Observations[0]
			blocker, err := testPool.Begin(t.Context())
			require.NoError(t, err)
			defer blocker.Rollback(context.Background())
			require.NoError(t, repository.AdvisoryLockSourceItemTx(t.Context(), blocker,
				repository.SourceItemLockKey(source, observation.SourceItemKey)))

			requestCtx, cancel := context.WithCancel(t.Context())
			defer cancel()
			result := make(chan error, 1)
			go func() {
				_, err := newService().ReportFromSource(requestCtx, request, source)
				result <- err
			}()

			var batchReceipt, reservedID string
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				err := testPool.QueryRow(t.Context(), `
SELECT batch_receipt::text, observations #>> '{0,ingest_id}'
FROM risk_ingest_batches WHERE source_ref=$1 AND result IS NULL`, source).Scan(&batchReceipt, &reservedID)
				assert.NoError(c, err)
				assert.NotEmpty(c, reservedID)
			}, 5*time.Second, 20*time.Millisecond)
			require.EventuallyWithT(t, func(c *assert.CollectT) {
				var blocked bool
				err := testPool.QueryRow(t.Context(), `
SELECT EXISTS(SELECT 1 FROM pg_stat_activity
              WHERE wait_event='advisory' AND query='SELECT pg_advisory_xact_lock($1)')`).Scan(&blocked)
				assert.NoError(c, err)
				assert.True(c, blocked)
			}, 5*time.Second, 20*time.Millisecond)
			var count int
			require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT count(*) FROM risk_observations WHERE source_ref=$1`, source).Scan(&count))
			require.Zero(t, count)
			cancel()
			select {
			case err := <-result:
				require.Error(t, err)
			case <-time.After(5 * time.Second):
				t.Fatal("request did not cancel during application")
			}

			require.NoError(t, blocker.Rollback(t.Context()))
			svc := newService()
			if method == "consumer" {
				// A changed mapping cannot apply old pinned input.
				_, err := testPool.Exec(t.Context(), `UPDATE risk_sources SET mapping_ref='changed-mapping' WHERE source_ref=$1`, source)
				require.NoError(t, err)
				require.NoError(t, svc.ResumeSourceReports(t.Context(), source, 10))
				require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT count(*) FROM risk_observations WHERE source_ref=$1`, source).Scan(&count))
				require.Zero(t, count)
				_, err = svc.ReportFromSource(t.Context(), request, source)
				require.ErrorIs(t, err, risk.ErrRiskSourceScope)

				_, err = testPool.Exec(t.Context(), `UPDATE risk_sources SET mapping_ref=$2 WHERE source_ref=$1`, source, sourceMapping)
				require.NoError(t, err)
				require.NoError(t, svc.ResumeSourceReports(t.Context(), source, 10))
				require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT count(*) FROM risk_observations WHERE source_ref=$1`, source).Scan(&count))
				require.Equal(t, 1, count)
			}
			recovered, err := svc.ReportFromSource(t.Context(), request, source)
			require.NoError(t, err)
			require.Equal(t, batchReceipt, recovered.BatchReceipt)
			require.Equal(t, reservedID, recovered.Items[0].IngestID)
			require.Equal(t, "accepted", recovered.Items[0].Result)
			require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT count(*) FROM risk_observations WHERE source_ref=$1`, source).Scan(&count))
			require.Equal(t, 1, count)
		})
	}
}

func TestRiskFeedConcurrentBatchesShareCanonicalEvent(t *testing.T) {
	source, first := seedReportBatch(t, "batch-one")
	shared := first.Observations[0]
	extra := shared
	extra.EventID = "second-" + extra.EventID
	extra.SourceItemKey += "-second-asset"
	second := model.RiskReportRequest{BatchID: "batch-two", Observations: []model.FindingObservationV1{shared, extra}}
	start := make(chan struct{})
	type outcome struct {
		result *model.RiskReportResult
		err    error
	}
	results := make(chan outcome, 8)
	for _, request := range []model.RiskReportRequest{first, second, first, second, first, second, first, second} {
		go func() {
			<-start
			response, err := newService().ReportFromSource(t.Context(), request, source)
			results <- outcome{response, err}
		}()
	}
	close(start)
	byLength := map[int]*model.RiskReportResult{}
	for range 8 {
		got := <-results
		require.NoError(t, got.err)
		size := len(got.result.Items)
		if previous := byLength[size]; previous != nil {
			require.Equal(t, previous, got.result)
		}
		byLength[size] = got.result
	}
	require.Len(t, byLength, 2)
	require.NotEqual(t, byLength[1].BatchReceipt, byLength[2].BatchReceipt)
	require.Equal(t, byLength[1].Items[0].IngestID, byLength[2].Items[0].IngestID)
	require.ElementsMatch(t, []string{"accepted", "duplicate"}, []string{byLength[1].Items[0].Result, byLength[2].Items[0].Result})
	require.Equal(t, "accepted", byLength[2].Items[1].Result)
	var batches, observations, riskEvents int
	require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT (SELECT count(*) FROM risk_ingest_batches WHERE source_ref=$1 AND result IS NOT NULL),
       (SELECT count(*) FROM risk_observations WHERE source_ref=$1),
       (SELECT count(*) FROM risk_events e JOIN risk_observations o ON o.risk_id=e.risk_id WHERE o.source_ref=$1)`,
		source).Scan(&batches, &observations, &riskEvents))
	require.Equal(t, 2, batches)
	require.Equal(t, 2, observations)
	require.Equal(t, 2, riskEvents)
}

func TestRiskFeedDoesNotProjectOrChangeOldOrganizationAfterMove(t *testing.T) {
	source, request := seedReportBatch(t, "original-organization")
	svc := newService()
	first, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	initial := first.Items[0]
	require.NotNil(t, initial.RiskRef)
	_, err = testPool.Exec(t.Context(), `
UPDATE risk_sources SET default_org_id=$2, allowed_org_ids=ARRAY[$2::bigint] WHERE source_ref=$1`, source, orgDID)
	require.NoError(t, err)

	request.BatchID = "another-batch"
	replay, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, "duplicate", replay.Items[0].Result)
	require.Equal(t, initial.IngestID, replay.Items[0].IngestID)
	require.Nil(t, replay.Items[0].RiskRef)

	request.BatchID = "new-observation"
	request.Observations[0].EventID = "new-" + request.Observations[0].EventID
	request.Observations[0].SourceSequence = "11"
	advanced, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, "held", advanced.Items[0].Result)
	require.Equal(t, "risk_scope_changed", advanced.Items[0].SafeReason)
	require.Nil(t, advanced.Items[0].RiskRef)
	var sequence string
	require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT latest_sequence FROM risk_source_records WHERE source_ref=$1`, source).Scan(&sequence))
	require.Equal(t, "10", sequence)
	history, err := svc.History(t.Context(), initial.RiskRef.ID, 1, 10, userM)
	require.NoError(t, err)
	require.Len(t, history.Items, 1)
}

func TestRiskFeedBatchFingerprintPreservesJSONIntegers(t *testing.T) {
	source, request := seedReportBatch(t, "large-integer")
	request.Observations[0].Attributes = json.RawMessage(`{"key":9007199254740992,"labels":{"b":"B","a":"A"}}`)
	svc := newService()
	first, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)

	request.Observations[0].Attributes = json.RawMessage(`{"labels":{"a":"A", "b":"B"},"key":9007199254740992}`)
	reordered, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, first, reordered)

	request.Observations[0].Attributes = json.RawMessage(`{"labels":{"a":"A","b":"B"},"key":9007199254740993}`)
	_, err = svc.ReportFromSource(t.Context(), request, source)
	require.ErrorIs(t, err, risk.ErrRiskOperationConflict)
}

func TestRiskFeedRetainsStoredHashForLegacyNegativeZero(t *testing.T) {
	source, request := seedReportBatch(t, "negative-zero")
	request.Observations[0].Attributes = json.RawMessage(`{"numeric":-0}`)
	svc := newService()
	original, err := svc.Report(t.Context(), request, userM)
	require.NoError(t, err)
	require.Equal(t, "accepted", original.Items[0].Result)
	legacyID := original.Items[0].IngestID

	recovered, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, "duplicate", recovered.Items[0].Result)
	require.Equal(t, legacyID, recovered.Items[0].IngestID)
	var conflicts int
	require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT count(*) FROM risk_ingest_conflicts WHERE source_ref=$1`, source).Scan(&conflicts))
	require.Zero(t, conflicts)
}

func TestRiskFeedLongIdentityKeepsFullKeys(t *testing.T) {
	source, base := seedReportBatch(t, "old-risk-long-asset")
	svc := newService()
	request := base
	first, err := svc.Report(t.Context(), request, userM)
	require.NoError(t, err)
	originalRisk := first.Items[0].RiskRef
	require.NotNil(t, originalRisk)
	continued := base.Observations[0]
	continued.EventID += "-long"
	continued.SourceSequence = "11"
	continued.FindingKey = strings.Repeat("核", 200)
	continued.AssetRef = &model.RiskAssetRef{Provider: "synthetic-assets", ExternalID: strings.Repeat("资", 200)}
	request.Observations = []model.FindingObservationV1{continued}
	replayed, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, originalRisk, replayed.Items[0].RiskRef)

	continued.EventID += "-new"
	continued.SourceItemKey += "-new"
	request.BatchID = "new-risk-long-asset"
	request.Observations = []model.FindingObservationV1{continued}
	created, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.NotNil(t, created.Items[0].RiskRef)
	detail, err := svc.Detail(t.Context(), created.Items[0].RiskRef.ID, userM)
	require.NoError(t, err)
	require.Equal(t, 200, len([]rune(detail.View.Title)))
	require.Equal(t, continued.FindingKey, detail.View.FindingKey)
	require.Equal(t, continued.AssetRef, detail.View.AssetRef)
}

func TestRiskFeedConflictingBatchesSerializeSharedEventsAcrossItemKeys(t *testing.T) {
	source, request := seedReportBatch(t, "left")
	leftA := request.Observations[0]
	leftA.EventID += "-a"
	leftA.SourceItemKey = "left-a"
	leftB := leftA
	leftB.EventID = request.Observations[0].EventID + "-b"
	leftB.SourceItemKey = "left-b"
	rightB, rightA := leftB, leftA
	rightB.SourceItemKey = "right-b"
	rightA.SourceItemKey = "right-a"
	left := model.RiskReportRequest{BatchID: "left", Observations: []model.FindingObservationV1{leftA, leftB}}
	right := model.RiskReportRequest{BatchID: "right", Observations: []model.FindingObservationV1{rightB, rightA}}

	// Pause each old would-be winner after its first insert so the subsequent
	// event claims would wait on each other without ordered event locking.
	const gateLeft, gateRight = int64(-1620001), int64(-1620002)
	_, err := testPool.Exec(t.Context(), fmt.Sprintf(`
CREATE FUNCTION zz_pause_conflicting_risk_batches() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.source_ref='%s' AND NEW.source_item_key='left-a' THEN
    PERFORM pg_advisory_xact_lock(%d);
  ELSIF NEW.source_ref='%s' AND NEW.source_item_key='right-b' THEN
    PERFORM pg_advisory_xact_lock(%d);
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER zz_pause_conflicting_risk_batches AFTER INSERT ON risk_observations
FOR EACH ROW EXECUTE FUNCTION zz_pause_conflicting_risk_batches()`,
		source, gateLeft, source, gateRight))
	require.NoError(t, err)
	t.Cleanup(func() {
		_, err := testPool.Exec(context.Background(), `
DROP TRIGGER zz_pause_conflicting_risk_batches ON risk_observations;
DROP FUNCTION zz_pause_conflicting_risk_batches()`)
		require.NoError(t, err)
	})
	blocker, err := testPool.Begin(t.Context())
	require.NoError(t, err)
	defer blocker.Rollback(context.Background())
	require.NoError(t, repository.AdvisoryLockSourceItemTx(t.Context(), blocker, gateLeft))
	require.NoError(t, repository.AdvisoryLockSourceItemTx(t.Context(), blocker, gateRight))

	type outcome struct {
		result *model.RiskReportResult
		err    error
	}
	results := make(chan outcome, 2)
	ctx, cancel := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancel()
	for _, input := range []model.RiskReportRequest{left, right} {
		go func() {
			result, err := newService().ReportFromSource(ctx, input, source)
			results <- outcome{result, err}
		}()
	}
	require.EventuallyWithT(t, func(c *assert.CollectT) {
		var waiting int
		err := testPool.QueryRow(t.Context(), `
SELECT count(*) FROM pg_stat_activity WHERE wait_event='advisory'
  AND (query LIKE 'INSERT INTO risk_observations%' OR query='SELECT pg_advisory_xact_lock($1)')`).Scan(&waiting)
		assert.NoError(c, err)
		assert.Equal(c, 2, waiting)
	}, 5*time.Second, 20*time.Millisecond)
	require.NoError(t, blocker.Rollback(t.Context()))

	first := <-results
	second := <-results
	require.NoError(t, first.err)
	require.NoError(t, second.err)
	if first.result.Items[0].Result == "quarantined" {
		first, second = second, first
	}
	require.NotEqual(t, first.result.BatchReceipt, second.result.BatchReceipt)
	require.Equal(t, "accepted", first.result.Items[0].Result)
	require.Equal(t, "accepted", first.result.Items[1].Result)
	require.Equal(t, "quarantined", second.result.Items[0].Result)
	require.Equal(t, "quarantined", second.result.Items[1].Result)
	require.Equal(t, first.result.Items[0].IngestID, second.result.Items[1].IngestID)
	require.Equal(t, first.result.Items[1].IngestID, second.result.Items[0].IngestID)
}

func TestRiskFeedWideLegacyObservationReplaysAndNewRiskHolds(t *testing.T) {
	source, request := seedReportBatch(t, "legacy-wide-identity")
	svc := newService()
	first, err := svc.Report(t.Context(), request, userM)
	require.NoError(t, err)
	require.NotNil(t, first.Items[0].RiskRef)

	wide := request.Observations[0]
	wide.EventID += "-wide"
	wide.SourceSequence = "11"
	wide.FindingKey = strings.Repeat("核", 201)
	wide.AssetRef = &model.RiskAssetRef{Provider: strings.Repeat("p", 101), ExternalID: strings.Repeat("资", 201)}
	request.Observations = []model.FindingObservationV1{wide}
	legacy, err := svc.Report(t.Context(), request, userM)
	require.NoError(t, err)
	require.Equal(t, "accepted", legacy.Items[0].Result)
	require.Equal(t, first.Items[0].RiskRef, legacy.Items[0].RiskRef)

	replay, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, "duplicate", replay.Items[0].Result)
	require.Equal(t, legacy.Items[0].IngestID, replay.Items[0].IngestID)
	var findingKey, provider, asset string
	require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT mirror ->> 'finding_key', mirror #>> '{asset_ref,provider}', mirror #>> '{asset_ref,external_id}'
FROM risk_source_records WHERE source_ref=$1 AND source_item_key=$2`, source, wide.SourceItemKey).
		Scan(&findingKey, &provider, &asset))
	require.Equal(t, wide.FindingKey, findingKey)
	require.Equal(t, wide.AssetRef.Provider, provider)
	require.Equal(t, wide.AssetRef.ExternalID, asset)

	fresh := wide
	fresh.EventID += "-new"
	fresh.SourceItemKey += "-new"
	request.BatchID = "wide-identity-on-new-risk"
	request.Observations = []model.FindingObservationV1{fresh}
	held, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, "held", held.Items[0].Result)
	require.Equal(t, "identity_too_long", held.Items[0].SafeReason)
	require.Nil(t, held.Items[0].RiskRef)
	heldReceipt, err := svc.IngestReceipt(t.Context(), held.Items[0].IngestID, userM)
	require.NoError(t, err)
	require.Equal(t, "held", heldReceipt.Processing)
	require.Equal(t, "identity_too_long", heldReceipt.SafeReason)

	replayed, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, held, replayed)
	var mirrors, events, pending int
	require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT (SELECT count(*) FROM risk_source_records WHERE source_ref=$1),
       (SELECT count(*) FROM risk_events WHERE risk_id=$2),
       (SELECT count(*) FROM risk_ingest_batches WHERE source_ref=$1 AND result IS NULL)`,
		source, first.Items[0].RiskRef.ID).Scan(&mirrors, &events, &pending))
	require.Equal(t, 1, mirrors)
	require.Equal(t, 2, events)
	require.Zero(t, pending)
}

func TestRiskFeedWideIdentityCannotOpenSuppressionSuccessor(t *testing.T) {
	source, request := seedReportBatch(t, "suppressed-identity-width")
	svc := newService()
	now := time.Now().UTC()
	request.Observations[0].ObservedAt = now.Add(-4 * time.Hour)
	first, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	risk := first.Items[0].RiskRef
	require.NotNil(t, risk)
	closed, err := svc.FalsePositive(t.Context(), model.RiskFalsePositiveRequest{
		ID: risk.ID, OperationID: "wide-successor-" + uniqueSuffix(), ExpectedRevision: "1",
		Reason: "Synthetic evidence excludes the first observation", SuppressionValidUntil: now.Add(time.Hour),
	}, userM)
	require.NoError(t, err)
	_, err = testPool.Exec(t.Context(), `UPDATE risk_suppressions SET
		created_at=$2, valid_until=$3 WHERE id=$1`, closed.Suppression.ID, now.Add(-3*time.Hour), now.Add(-time.Hour))
	require.NoError(t, err)

	next := request.Observations[0]
	next.EventID += "-wide-successor"
	next.SourceSequence = "11"
	next.ObservedAt = now.Add(-5 * time.Minute)
	next.AssetRef = &model.RiskAssetRef{Provider: "synthetic", ExternalID: strings.Repeat("资", 201)}
	request.BatchID = "wide-successor"
	request.Observations = []model.FindingObservationV1{next}
	held, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, "held", held.Items[0].Result)
	require.Equal(t, "identity_too_long", held.Items[0].SafeReason)
	require.Nil(t, held.Items[0].RiskRef)
	var current int64
	require.NoError(t, testPool.QueryRow(t.Context(), `
SELECT f.current_cycle_id FROM risk_families f JOIN risk_items ri ON ri.family_id=f.id WHERE ri.id=$1`, risk.ID).Scan(&current))
	require.Equal(t, risk.ID, current)

	next.EventID += "-valid"
	next.SourceSequence = "12"
	next.AssetRef.ExternalID = "synthetic-valid"
	request.BatchID = "valid-successor"
	request.Observations = []model.FindingObservationV1{next}
	resumed, err := svc.ReportFromSource(t.Context(), request, source)
	require.NoError(t, err)
	require.Equal(t, "accepted", resumed.Items[0].Result)
	require.NotNil(t, resumed.Items[0].RiskRef)
	require.NotEqual(t, risk.ID, resumed.Items[0].RiskRef.ID)
}

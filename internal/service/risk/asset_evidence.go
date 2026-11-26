package risk

import (
	"context"

	"github.com/google/uuid"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/riskasset"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// AssetEvidenceResult is a current directory read for an already visible risk.
type AssetEvidenceResult struct {
	Status   string              `json:"status"`
	Evidence *riskasset.Evidence `json:"evidence,omitempty"`
}

func (s *Service) AssetEvidence(ctx context.Context, riskID, actorID int64, directory riskasset.Port) (AssetEvidenceResult, error) {
	if riskID <= 0 || actorID <= 0 {
		return AssetEvidenceResult{}, ErrRiskNotFound
	}
	row, _, _, err := repository.GetRiskItemVisible(ctx, s.db, riskID, actorID)
	if err != nil {
		return AssetEvidenceResult{}, err
	}
	if row.AssetProvider == nil || row.AssetExternalID == nil {
		return AssetEvidenceResult{Status: "not_linked"}, nil
	}
	if directory == nil {
		return AssetEvidenceResult{Status: "disabled"}, nil
	}
	requestID, err := uuid.NewRandom()
	if err != nil {
		return AssetEvidenceResult{Status: string(riskasset.StatusUnavailable)}, nil
	}
	ref := riskasset.Ref{Provider: *row.AssetProvider, ExternalID: *row.AssetExternalID}
	result, err := directory.Lookup(ctx, riskasset.LookupRequest{AssetRef: ref, RequestID: requestID.String()})
	if ctx.Err() != nil {
		return AssetEvidenceResult{}, ctx.Err()
	}
	switch result.Status {
	case riskasset.StatusOK, riskasset.StatusStale:
		if err == nil && result.Evidence != nil && result.Evidence.AssetRef == ref {
			return AssetEvidenceResult{Status: string(result.Status), Evidence: result.Evidence}, nil
		}
	case riskasset.StatusNotFound, riskasset.StatusTimeout:
		return AssetEvidenceResult{Status: string(result.Status)}, nil
	}
	return AssetEvidenceResult{Status: string(riskasset.StatusUnavailable)}, nil
}

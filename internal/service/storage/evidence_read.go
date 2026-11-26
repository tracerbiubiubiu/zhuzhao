package storage

import "context"

// UseRiskEvidenceRouteReader injects the current risk detail route authority at startup.
func (s *Service) UseRiskEvidenceRouteReader(reader func(context.Context, int64, []int64) (map[int64]bool, error)) {
	s.riskEvidenceRoutes = reader
}

func (s *Service) hiddenEvidenceRoutes(ctx context.Context, actorID int64, keys []string) (map[string]bool, error) {
	subjects, err := s.repo.RiskEvidenceSubjects(ctx, keys)
	if err != nil || len(subjects) == 0 {
		return nil, err
	}
	ids := make(map[int64]struct{})
	for _, risks := range subjects {
		for _, id := range risks {
			ids[id] = struct{}{}
		}
	}
	var granted map[int64]bool
	if s.riskEvidenceRoutes != nil {
		riskIDs := make([]int64, 0, len(ids))
		for id := range ids {
			riskIDs = append(riskIDs, id)
		}
		granted, err = s.riskEvidenceRoutes(ctx, actorID, riskIDs)
		if err != nil {
			return nil, err
		}
	}
	hidden := make(map[string]bool)
	for key, risks := range subjects {
		for _, id := range risks {
			if !granted[id] {
				hidden[key] = true
				break
			}
		}
	}
	return hidden, nil
}

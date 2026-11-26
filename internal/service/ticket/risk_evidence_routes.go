package ticket

import (
	"context"
	"strconv"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// RiskEvidenceRouteReader projects the same current L1 detail-route policy
// that protects ordinary risk reads. The caller separately checks risk and ticket scope.
func (s *Service) RiskEvidenceRouteReader(modelText string) func(context.Context, int64, []int64) (map[int64]bool, error) {
	return func(ctx context.Context, actor int64, ids []int64) (map[int64]bool, error) {
		roles, policies, err := repository.NewUserRepo(s.db).CurrentIAMAuthorization(ctx, actor)
		if err != nil {
			return nil, err
		}
		routes := make([]ticketPolicyRoute, len(ids))
		for i, id := range ids {
			routes[i] = ticketPolicyRoute{path: "/api/v1/risk-items/" + strconv.FormatInt(id, 10), method: "GET"}
		}
		grants, err := commandRouteGrants(modelText, roles, policies, routes)
		if err != nil {
			return nil, err
		}
		byRisk := make(map[int64]bool, len(ids))
		for i, id := range ids {
			byRisk[id] = grants[i]
		}
		return byRisk, nil
	}
}

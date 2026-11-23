package audit

import "strings"

// RiskRoutePrefix identifies the object-authorized risk API in operation logs.
const RiskRoutePrefix = "/api/v1/risk-items"

// IsRiskRoute matches the risk collection and its descendants on a path boundary.
func IsRiskRoute(path string) bool {
	return path == RiskRoutePrefix || strings.HasPrefix(path, RiskRoutePrefix+"/")
}

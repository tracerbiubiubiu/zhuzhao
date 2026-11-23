package audit

import "strings"

// RiskRoutePrefix identifies the object-authorized risk API in operation logs.
const RiskRoutePrefix = "/api/v1/risk-items"

// IsRiskRoute matches the risk collection and its descendants on a path boundary.
func IsRiskRoute(path string) bool {
	return path == RiskRoutePrefix || strings.HasPrefix(path, RiskRoutePrefix+"/") ||
		RiskEvidenceRoutePath(path) != ""
}

// RiskEvidenceRoutePath returns the audit template of a shared file or attachment
// read route whose payload or object key may refer to risk evidence.
func RiskEvidenceRoutePath(path string) string {
	if strings.HasPrefix(path, "/api/v1/files/") {
		return "/api/v1/files/*key"
	}
	rest, ok := strings.CutPrefix(path, "/api/v1/tickets/")
	if ok {
		if id, action, ok := strings.Cut(rest, "/"); ok && id != "" && action == "attachments" {
			return "/api/v1/tickets/:id/attachments"
		}
	}
	return ""
}

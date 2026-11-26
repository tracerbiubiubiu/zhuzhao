// Package referencequery defines the public v1 synthetic asset-directory wire contract.
package referencequery

import (
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

const (
	Profile       = "synthetic.asset-directory.v1"
	QueryID       = "reference.asset.list"
	Version       = "1"
	ExtensionID   = "reference.assets"
	AppPath       = "/v1/reference-assets/query"
	AdmissionPath = "/internal/extensions/reference-assets/admission"
	QueryPath     = "/api/v1/extensions/reference-assets"
	ConnectorPath = "/v1/query"
)

type Parameters struct {
	OwnerOrgRefs []string `json:"owner_org_refs,omitempty"`
	Status       []string `json:"status,omitempty"`
}
type QueryRequest struct {
	QueryID         string     `json:"query_id"`
	ContractVersion string     `json:"contract_version"`
	SourceID        string     `json:"source_id"`
	Parameters      Parameters `json:"parameters"`
	Cursor          string     `json:"cursor,omitempty"`
	Limit           int        `json:"limit"`
	RequestedFields []string   `json:"requested_fields"`
}
type QueryPage struct {
	Items          []map[string]any `json:"items"`
	NextCursor     string           `json:"next_cursor,omitempty"`
	Complete       bool             `json:"complete"`
	SourceID       string           `json:"source_id"`
	ObservedAt     string           `json:"observed_at"`
	FetchedAt      string           `json:"fetched_at"`
	MappingVersion string           `json:"mapping_version"`
	SourceRevision string           `json:"source_revision,omitempty"`
	Warnings       []string         `json:"warnings"`
}
type Invocation struct {
	RequestRef      string       `json:"request_ref"`
	Audience        string       `json:"audience"`
	CapabilityID    string       `json:"capability_id"`
	ContractVersion string       `json:"contract_version"`
	ConfigRevision  string       `json:"config_revision"`
	RequestID       string       `json:"request_id"`
	Method          string       `json:"method"`
	Path            string       `json:"path"`
	Query           QueryRequest `json:"query"`
}
type Admission struct {
	Query QueryRequest `json:"query"`
}

type Failure struct {
	Status       int    `json:"-"`
	Code         string `json:"reason"`
	RetryAfterMS *int   `json:"retry_after_ms,omitempty"`
}

func (f *Failure) Error() string { return f.Code }

func validText(s string, max int) bool {
	return len(s) > 0 && len(s) <= max && utf8.ValidString(s) && strings.TrimSpace(s) == s && strings.IndexFunc(s, unicode.IsControl) < 0
}
func validID(s string) bool {
	if len(s) == 0 || s[0] == '0' {
		return false
	}
	for i := range len(s) {
		if s[i] < '0' || s[i] > '9' {
			return false
		}
	}
	return true
}
func validStatus(s string) bool {
	return slices.Contains([]string{"healthy", "degraded", "down", "unknown"}, s)
}
func validField(s string) bool {
	return slices.Contains([]string{"device_id", "name", "owner_org_ref", "category", "status", "observed_at", "stale"}, s)
}
func validSet(values []string, max int, accept func(string) bool) bool {
	if len(values) > max {
		return false
	}
	seen := make(map[string]bool, len(values))
	for _, v := range values {
		if seen[v] || !accept(v) {
			return false
		}
		seen[v] = true
	}
	return true
}

// ValidateInput checks a user filter, which can omit the organization filter.
func ValidateInput(q QueryRequest) error {
	if q.ContractVersion != Version || q.QueryID != QueryID {
		return &Failure{Status: 409, Code: "contract_mismatch"}
	}
	if !validText(q.SourceID, 256) || q.Limit < 1 || q.Limit > 50 || (q.Cursor != "" && !validText(q.Cursor, 4096)) ||
		!validSet(q.Parameters.OwnerOrgRefs, 256, validID) || !validSet(q.Parameters.Status, 4, validStatus) ||
		len(q.RequestedFields) == 0 || !validSet(q.RequestedFields, 7, validField) {
		return &Failure{Status: 400, Code: "invalid_input"}
	}
	return nil
}

// ValidateRequest also requires the explicit authorized scope sent to a connector.
func ValidateRequest(q QueryRequest) error {
	if err := ValidateInput(q); err != nil {
		return err
	}
	if len(q.Parameters.OwnerOrgRefs) == 0 {
		return &Failure{Status: 400, Code: "invalid_input"}
	}
	return nil
}

func timestamp(s string) bool { _, err := time.Parse(time.RFC3339Nano, s); return err == nil }
func validValue(k string, v any, q QueryRequest) bool {
	s, isString := v.(string)
	switch k {
	case "device_id":
		return isString && validID(s)
	case "owner_org_ref":
		return isString && slices.Contains(q.Parameters.OwnerOrgRefs, s)
	case "name", "category":
		return isString
	case "status":
		return isString && validStatus(s) && (len(q.Parameters.Status) == 0 || slices.Contains(q.Parameters.Status, s))
	case "stale":
		_, ok := v.(bool)
		return ok
	case "observed_at":
		return v == nil || isString && timestamp(s)
	default:
		return false
	}
}

// ValidatePage checks the exact requested projection and scope. Callers that
// consume untrusted rows must request the owner and device ID before projecting.
func ValidatePage(q QueryRequest, p QueryPage) error {
	if err := ValidateRequest(q); err != nil {
		return err
	}
	invalid := &Failure{Status: 409, Code: "contract_mismatch"}
	if p.SourceID != q.SourceID || p.Items == nil || len(p.Items) > q.Limit || !timestamp(p.ObservedAt) ||
		!timestamp(p.FetchedAt) || !validText(p.MappingVersion, 256) || p.Warnings == nil ||
		p.Complete && p.NextCursor != "" || !p.Complete && p.NextCursor == "" && len(p.Warnings) == 0 ||
		p.NextCursor != "" && !validText(p.NextCursor, 4096) {
		return invalid
	}
	for _, w := range p.Warnings {
		if !validText(w, 128) {
			return invalid
		}
	}
	var last string
	for _, row := range p.Items {
		if len(row) != len(q.RequestedFields) {
			return invalid
		}
		for _, k := range q.RequestedFields {
			v, ok := row[k]
			if !ok || !validValue(k, v, q) {
				return invalid
			}
		}
		if id, ok := row["device_id"].(string); ok {
			if len(id) < len(last) || len(id) == len(last) && id <= last {
				return invalid
			}
			last = id
		}
	}
	return nil
}

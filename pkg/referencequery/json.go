package referencequery

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"unicode/utf8"
)

// Verify exact snake_case keys, unique members, bounded nesting and a single
// object before decoding. encoding/json alone folds case and accepts repeats.
func validObject(b []byte) bool {
	if !utf8.Valid(b) {
		return false
	}
	d := json.NewDecoder(bytes.NewReader(b))
	token, err := d.Token()
	if err != nil || token != json.Delim('{') {
		return false
	}
	if !members(d, '{', 1) {
		return false
	}
	_, err = d.Token()
	return err == io.EOF
}
func members(d *json.Decoder, open json.Delim, depth int) bool {
	if depth > 12 {
		return false
	}
	seen := map[string]bool{}
	for d.More() {
		if open == '{' {
			token, err := d.Token()
			k, ok := token.(string)
			if err != nil || !ok || k == "" || seen[k] {
				return false
			}
			for _, c := range k {
				if c != '_' && (c < 'a' || c > 'z') && (c < '0' || c > '9') {
					return false
				}
			}
			seen[k] = true
		}
		token, err := d.Token()
		if err != nil {
			return false
		}
		if nested, ok := token.(json.Delim); ok && !members(d, nested, depth+1) {
			return false
		}
	}
	_, err := d.Token()
	return err == nil
}

// UnmarshalJSON rejects malformed optional values before Go can coerce null
// to an omitted filter. Required fields must be present even for an empty scope.
func (q *QueryRequest) UnmarshalJSON(data []byte) error {
	type plain QueryRequest
	var raw struct {
		QueryID         *string                     `json:"query_id"`
		ContractVersion *string                     `json:"contract_version"`
		SourceID        *string                     `json:"source_id"`
		Parameters      *map[string]json.RawMessage `json:"parameters"`
		RequestedFields *[]string                   `json:"requested_fields"`
		Limit           *int                        `json:"limit"`
		Cursor          json.RawMessage             `json:"cursor"`
	}
	invalid := errors.New("invalid query request")
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil || raw.QueryID == nil || raw.ContractVersion == nil || raw.SourceID == nil ||
		raw.Parameters == nil || raw.RequestedFields == nil || raw.Limit == nil {
		return invalid
	}
	for k, v := range *raw.Parameters {
		if (k != "owner_org_refs" && k != "status") || bytes.Equal(v, []byte("null")) {
			return invalid
		}
	}
	var result plain
	if err := json.Unmarshal(data, &result); err != nil {
		return invalid
	}
	if raw.Cursor != nil && result.Cursor == "" {
		return invalid
	}
	*q = QueryRequest(result)
	return nil
}

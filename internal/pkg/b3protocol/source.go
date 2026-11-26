package b3protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

type SourceDelivery struct {
	SchemaVersion string          `json:"schema_version"`
	DeliveryID    string          `json:"delivery_id"`
	ConsumerID    string          `json:"consumer_id"`
	Event         json.RawMessage `json:"event"`
}

type SourceAck struct {
	SchemaVersion  string `json:"schema_version"`
	DeliveryID     string `json:"delivery_id"`
	Source         string `json:"source"`
	EventID        string `json:"event_id"`
	PayloadSHA256  string `json:"payload_sha256"`
	StageCommitRef string `json:"stage_commit_ref"`
	State          string `json:"state"`
}

type SourceCausality struct {
	RootID             string   `json:"root_id"`
	BudgetRef          string   `json:"root_budget_ref"`
	ParentEventID      string   `json:"parent_event_id,omitempty"`
	ParentInvocationID string   `json:"parent_invocation_id,omitempty"`
	Depth              int      `json:"depth"`
	RulePath           []string `json:"rule_path"`
}

type SourceEvent struct {
	SpecVersion string `json:"specversion"`
	ID          string `json:"id"`
	Source      string `json:"source"`
	Type        string `json:"type"`
	Subject     string `json:"subject"`
	Time        string `json:"time"`
	ContentType string `json:"datacontenttype"`
	Data        struct {
		StreamID         string          `json:"stream_id"`
		Cursor           string          `json:"cursor"`
		Generation       string          `json:"generation"`
		DocumentID       string          `json:"document_id"`
		DocumentRevision string          `json:"document_revision"`
		ChangedFields    []string        `json:"changed_fields,omitempty"`
		Values           map[string]any  `json:"event_values,omitempty"`
		OperationID      string          `json:"operation_id"`
		Causality        SourceCausality `json:"causality"`
	} `json:"data"`
}

var fieldName = regexp.MustCompile("^[a-z][a-z0-9_]{0,62}$")

func PositiveNumber(value string) bool {
	n, err := strconv.ParseInt(value, 10, 64)
	return err == nil && n > 0 && strconv.FormatInt(n, 10) == value
}

func ValidFields(fields []string, limit int) bool {
	if len(fields) == 0 || len(fields) > limit {
		return false
	}
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if !fieldName.MatchString(field) || seen[field] {
			return false
		}
		seen[field] = true
	}
	return true
}

func ValidType(name string) bool { return fieldName.MatchString(name) }

func ValidateSource(e SourceEvent, typeName, generation string) error {
	d, c := e.Data, e.Data.Causality
	if e.SpecVersion != "1.0" || e.ContentType != "application/json" ||
		!ValidID(e.ID) || e.Source != "urn:zhuzhao:activelist:"+typeName ||
		(e.Type != "al.document.created.v1" && e.Type != "al.document.updated.v1") ||
		e.Subject != typeName+"/"+d.DocumentID || d.StreamID != "activelist-stream-"+typeName+"-"+generation ||
		d.Generation != generation || !PositiveNumber(d.Cursor) || !PositiveNumber(d.DocumentID) ||
		!PositiveNumber(d.DocumentRevision) || !ValidID(d.OperationID) ||
		!ValidID(c.RootID) || !ValidID(c.BudgetRef) || c.Depth < 0 || c.Depth > 8 ||
		c.RulePath == nil || len(c.RulePath) > 8 {
		return errors.New("invalid ActiveList source event")
	}
	if _, err := time.Parse(time.RFC3339Nano, e.Time); err != nil {
		return errors.New("invalid ActiveList event timestamp")
	}
	for _, parent := range []string{c.ParentEventID, c.ParentInvocationID} {
		if parent != "" && !ValidID(parent) {
			return errors.New("invalid ActiveList event causality")
		}
	}
	for _, rule := range c.RulePath {
		if !ValidID(rule) {
			return errors.New("invalid ActiveList event rule path")
		}
	}
	if len(d.ChangedFields) > 0 && !ValidFields(d.ChangedFields, 32) {
		return errors.New("invalid ActiveList changed fields")
	}
	if len(d.Values) > 8 {
		return errors.New("invalid ActiveList event field count")
	}
	for key, value := range d.Values {
		if !fieldName.MatchString(key) || !validEventScalar(value) {
			return errors.New("invalid ActiveList event field")
		}
	}
	return nil
}

func validEventScalar(value any) bool {
	switch v := value.(type) {
	case nil, bool:
		return true
	case string:
		return utf8.RuneCountInString(v) <= 1024
	case json.Number:
		n, err := v.Int64()
		return err == nil && n >= -(1<<53-1) && n <= 1<<53-1 && strconv.FormatInt(n, 10) == string(v)
	default:
		return false
	}
}

// SourceHash uses compact sorted UTF-8 for the source protocol's ASCII keys
// and bounded integer values. It hashes the fact alone, without the delivery.
func SourceHash(raw json.RawMessage) (string, error) {
	var value map[string]any
	if err := DecodeBytes(raw, &value); err != nil {
		return "", err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return "", err
	}
	text := strings.TrimSuffix(buf.String(), "\n")
	var canonical strings.Builder
	for i := 0; i < len(text); i++ {
		if text[i] == '\\' && i+1 < len(text) {
			if strings.HasPrefix(text[i:], `\u2028`) || strings.HasPrefix(text[i:], `\u2029`) {
				if text[i+5] == '8' {
					canonical.WriteRune(0x2028)
				} else {
					canonical.WriteRune(0x2029)
				}
				i += 5
				continue
			}
			canonical.WriteByte(text[i])
			i++
		}
		canonical.WriteByte(text[i])
	}
	hash := sha256.Sum256([]byte(canonical.String()))
	return hex.EncodeToString(hash[:]), nil
}

type SourceRead struct {
	TypeName      string          `json:"type_name"`
	DocumentID    string          `json:"document_id"`
	Generation    string          `json:"generation"`
	Fields        []string        `json:"fields"`
	EventID       string          `json:"event_id"`
	EventRevision string          `json:"event_revision"`
	Causality     SourceCausality `json:"causality"`
	GrantRef      string          `json:"grant_ref"`
	GrantRevision string          `json:"grant_revision"`
	SubjectID     string          `json:"subject_id"`
	RuleID        string          `json:"rule_id"`
}

func (e SourceEvent) CanDerive(rule string) bool {
	return e.Data.Causality.Depth < 8 && !slices.Contains(e.Data.Causality.RulePath, rule)
}

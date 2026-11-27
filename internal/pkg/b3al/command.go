// Package b3al binds the selected ActiveList task to its current service grant.
package b3al

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
)

// Command comes from the durable original operation and its claimed execution.
type Command struct {
	OperationID, InvocationID, SubjectType, SubjectID                      string
	AuthorizationRef, AuthorizationRevision                                string
	TypeName, DocumentID, Generation, ExpectedRevision, Fence              string
	RootID, RootBudgetRef, ParentInvocationID, ParentEventID               string
	SourceEventID, SourceEventRevision, ExpectedDefinitionHash, Entrypoint string
	Depth                                                                  int
	RulePath, Fields                                                       []string
	Patch                                                                  map[string]any
	SubmittedAt, ExpiresAt                                                 time.Time
}

// An empty DocumentID selects the configured row-anchor profile. ActiveList
// resolves that grant reference and checks the anchor on the locked document.
type Grant struct {
	SubjectType, SubjectID, AuthorizationRef, AuthorizationRevision string
	TypeName, Generation, DocumentID                                string
	ReadFields, WriteFields                                         []string
}

var (
	digest     = regexp.MustCompile("^[a-f0-9]{64}$")
	ErrInvalid = errors.New("invalid ActiveList operation")
	ErrDenied  = errors.New("ActiveList grant does not permit this operation")
)

func (c Command) selected(write bool) []string {
	if write {
		return slices.Sorted(maps.Keys(c.Patch))
	}
	return c.Fields
}

func Authorize(c Command, g Grant, write bool) error {
	if g.SubjectType != c.SubjectType || g.SubjectID == "" || g.SubjectID != c.SubjectID || g.AuthorizationRef == "" || g.AuthorizationRef != c.AuthorizationRef || g.AuthorizationRevision != c.AuthorizationRevision || g.Generation != c.Generation || g.TypeName != c.TypeName || (g.DocumentID != "" && g.DocumentID != c.DocumentID) {
		return ErrDenied
	}
	allowed := g.ReadFields
	if write {
		allowed = g.WriteFields
	}
	fields := c.selected(write)
	if !b3protocol.ValidFields(fields, 32) || !b3protocol.ValidFields(allowed, 32) {
		return ErrDenied
	}
	for _, f := range fields {
		if !slices.Contains(allowed, f) {
			return ErrDenied
		}
	}
	return nil
}

// Scalar numbers in this profile are exact interoperable JSON integers.
func scalar(v any) (any, bool) {
	const largest = int64(1<<53 - 1)
	switch n := v.(type) {
	case nil, bool:
		return v, true
	case string:
		return n, utf8.ValidString(n) && utf8.RuneCountInString(n) <= 1024
	case json.Number:
		// Decimal/exponent forms are accepted only if their exact value is integral.
		return exactNumber(string(n))
	case float64:
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Trunc(n) != n || math.Abs(n) > float64(largest) {
			return nil, false
		}
		return int64(n), true
	case int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		raw, _ := json.Marshal(n)
		value, err := strconv.ParseInt(string(raw), 10, 64)
		return value, err == nil && value >= -largest && value <= largest
	default:
		return nil, false
	}
}

// canonicalBytes implements the restricted reference projection. Map keys are
// schema-constrained ASCII. Go's JSON encoder otherwise escapes U+2028/U+2029
// even with HTML escaping disabled, unlike the contract's JSON.stringify.
func canonicalBytes(value map[string]any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(value); err != nil {
		return nil, err
	}
	raw := bytes.TrimSuffix(buf.Bytes(), []byte{'\n'})
	out := make([]byte, 0, len(raw))
	for i := 0; i < len(raw); i++ {
		if raw[i] == '\\' && i+1 < len(raw) {
			if bytes.HasPrefix(raw[i:], []byte(`\u2028`)) || bytes.HasPrefix(raw[i:], []byte(`\u2029`)) {
				out = append(out, 0xe2, 0x80, 0xa8+(raw[i+5]-'8'))
				i += 5
				continue
			}
			out = append(out, raw[i], raw[i+1])
			i++
			continue
		}
		out = append(out, raw[i])
	}
	return out, nil
}

// ValidateDocumentUpdate checks only the frozen inputs. The current subject,
// grant state and row policy are checked when the operation is executed.
func ValidateDocumentUpdate(input b3protocol.DocumentUpdate) error {
	c := Command{
		TypeName: input.TypeName, DocumentID: input.DocumentID, Generation: input.Generation,
		ExpectedRevision: input.ExpectedRevision, Patch: input.Patch,
		AuthorizationRef: input.GrantRef, AuthorizationRevision: input.GrantRevision,
	}
	if !c.validTarget() {
		return ErrInvalid
	}
	_, err := c.updatePatch(c.selected(true))
	return err
}

func (c Command) validTarget() bool {
	return b3protocol.ValidID(c.AuthorizationRef) && b3protocol.ValidType(c.TypeName) &&
		b3protocol.PositiveNumber(c.DocumentID) && b3protocol.PositiveNumber(c.Generation) &&
		b3protocol.PositiveNumber(c.AuthorizationRevision)
}

func (c Command) updatePatch(fields []string) (map[string]any, error) {
	if !b3protocol.PositiveNumber(c.ExpectedRevision) || !b3protocol.ValidFields(fields, 32) {
		return nil, ErrInvalid
	}
	patch := make(map[string]any, len(fields))
	for _, f := range fields {
		v, ok := scalar(c.Patch[f])
		if !ok {
			return nil, ErrInvalid
		}
		patch[f] = v
	}
	return patch, nil
}

func (c Command) envelope(now time.Time, path string) (map[string]any, error) {
	write, lookup := path != getPath, path == receiptPath
	if !b3protocol.ValidID(c.OperationID) || !b3protocol.ValidID(c.InvocationID) || !b3protocol.ValidID(c.RootID) || !b3protocol.ValidID(c.RootBudgetRef) || !c.validTarget() || !b3protocol.PositiveNumber(c.Fence) {
		return nil, ErrInvalid
	}
	if (write && c.SubjectType != "human") || (!write && c.SubjectType != "service") || strings.TrimSpace(c.SubjectID) == "" || !utf8.ValidString(c.SubjectID) || utf8.RuneCountInString(c.SubjectID) > 128 || c.SubmittedAt.IsZero() || c.SubmittedAt.After(now) || !c.ExpiresAt.After(c.SubmittedAt) || (!lookup && !c.ExpiresAt.After(now)) {
		return nil, ErrInvalid
	}
	if !slices.Contains([]string{"http", "internal", "ticket", "schedule", "subscription"}, c.Entrypoint) || c.Depth < 0 || c.Depth > 8 || len(c.RulePath) > 8 {
		return nil, ErrInvalid
	}
	for _, key := range []string{c.ParentInvocationID, c.ParentEventID, c.SourceEventID} {
		if key != "" && !b3protocol.ValidID(key) {
			return nil, ErrInvalid
		}
	}
	seen := map[string]bool{}
	for _, key := range c.RulePath {
		if !b3protocol.ValidID(key) || seen[key] {
			return nil, ErrInvalid
		}
		seen[key] = true
	}
	fields := c.selected(write)
	target := map[string]any{"owner": "activelist", "resource_type": "activelist_document", "resource_id": c.DocumentID, "type_name": c.TypeName, "generation": c.Generation}
	params := map[string]any{"document_id": c.DocumentID, "type_name": c.TypeName, "generation": c.Generation}
	action, kind, claims := "get", "al_read_request", "read_admission_claims"
	if write {
		if len(c.Fields) > 0 || c.SourceEventID != "" || c.SourceEventRevision != "" {
			return nil, ErrInvalid
		}
		patch, err := c.updatePatch(fields)
		if err != nil {
			return nil, err
		}
		params["patch"] = patch
		params["expected_revision"] = c.ExpectedRevision
		action, kind, claims = "update", "al_execution_request", "admission_claims"
	} else {
		if !b3protocol.ValidFields(fields, 32) || len(c.Patch) > 0 || c.ExpectedRevision != "" || (c.SourceEventID == "") != (c.SourceEventRevision == "") || c.SourceEventRevision != "" && !b3protocol.PositiveNumber(c.SourceEventRevision) {
			return nil, ErrInvalid
		}
		params["fields"] = fields
		if c.SourceEventID != "" {
			params["source_event_id"] = c.SourceEventID
		}
	}
	capability := "activelist.document." + action
	subject := map[string]any{"subject_type": c.SubjectType, "subject_id": c.SubjectID}
	projection := map[string]any{"capability_id": capability, "contract_version": "1", "subject": subject, "target": target, "params": params}
	if c.ExpectedDefinitionHash != "" {
		if !digest.MatchString(c.ExpectedDefinitionHash) {
			return nil, ErrInvalid
		}
		projection["expected_definition_hash"] = c.ExpectedDefinitionHash
	}
	raw, err := canonicalBytes(projection)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(raw)
	hash := hex.EncodeToString(sum[:])
	principal := maps.Clone(subject)
	principal["authorization_ref"] = c.AuthorizationRef
	principal["authorization_revision"] = c.AuthorizationRevision
	rulePath := c.RulePath
	if rulePath == nil {
		rulePath = []string{}
	}
	causality := map[string]any{"root_id": c.RootID, "root_budget_ref": c.RootBudgetRef, "depth": c.Depth, "rule_path": rulePath}
	if c.ParentEventID != "" {
		causality["parent_event_id"] = c.ParentEventID
	}
	if c.ParentInvocationID != "" {
		causality["parent_invocation_id"] = c.ParentInvocationID
	}
	command := map[string]any{"schema_version": "1", "kind": "command", "operation_id": c.OperationID, "invocation_id": c.InvocationID, "capability_id": capability, "contract_version": "1", "canonical_input_hash": hash, "principal": principal, "target": target, "params": params, "causality": causality, "submitted_at": c.SubmittedAt.UTC(), "expires_at": c.ExpiresAt.UTC(), "entrypoint": c.Entrypoint}
	if c.ExpectedDefinitionHash != "" {
		command["expected_definition_hash"] = c.ExpectedDefinitionHash
	}
	expires := now.Add(30 * time.Second)
	if !lookup && c.ExpiresAt.Before(expires) {
		expires = c.ExpiresAt
	}
	purpose := "al_document_" + action
	if lookup {
		kind, claims, purpose = "al_receipt_request", "receipt_admission_claims", "al_effect_receipt"
	}
	admission := map[string]any{"schema_version": "1", "kind": claims, "purpose": purpose, "audience": "activelist", "operation_id": c.OperationID, "canonical_input_hash": hash, "authorization_ref": c.AuthorizationRef, "authorization_revision": c.AuthorizationRevision, "target": target, "authorized_fields": fields, "fence": c.Fence, "issued_at": now.UTC(), "expires_at": expires.UTC()}
	return map[string]any{"schema_version": "1", "kind": kind, "command": command, "admission": admission}, nil
}

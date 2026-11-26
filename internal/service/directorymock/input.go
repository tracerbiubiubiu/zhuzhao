// Package directorymock applies synthetic department batches to an isolated projection.
package directorymock

import (
	"errors"
	"io"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	jsonv2 "encoding/json/v2"
)

const (
	maxBatchBytes = 1 << 20
	maxPages      = 32
	maxRecords    = 100
)

var (
	ErrInvalidBatch     = errors.New("invalid_mock_batch")
	ErrBatchConflict    = errors.New("batch_content_conflict")
	ErrBaselineConflict = errors.New("batch_baseline_conflict")
	refPattern          = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
)

type Batch struct {
	ProviderRef      string `json:"provider_ref"`
	Domain           string `json:"domain"`
	Mode             string `json:"mode"`
	ScopeRef         string `json:"scope_ref"`
	BatchID          string `json:"batch_id"`
	ExpectedBaseline string `json:"expected_baseline"`
	SuccessCursor    string `json:"success_cursor"`
	Pages            []Page `json:"pages"`
}

type Page struct {
	Cursor     string       `json:"cursor"`
	NextCursor string       `json:"next_cursor"`
	Done       bool         `json:"done"`
	Error      string       `json:"error,omitempty"`
	Records    []Department `json:"records,omitempty"`
}

type Department struct {
	ExternalID     string `json:"external_id"`
	DisplayName    string `json:"display_name"`
	SourceRevision int64  `json:"source_revision"`
}

// ReadBatch reads one bounded JSON object and rejects ambiguous field names.
func ReadBatch(input io.Reader) (Batch, error) {
	var batch Batch
	reader := &io.LimitedReader{R: input, N: maxBatchBytes + 1}
	if err := jsonv2.UnmarshalRead(reader, &batch, jsonv2.RejectUnknownMembers(true)); err != nil || reader.N == 0 {
		return Batch{}, ErrInvalidBatch
	}
	return batch, batch.validate()
}

func (batch Batch) validate() error {
	if batch.ProviderRef != "mock-directory" || batch.Domain != "departments" || batch.Mode != "full" ||
		!validRef(batch.ScopeRef) || !validRef(batch.BatchID) || !validRef(batch.SuccessCursor) ||
		(batch.ExpectedBaseline != "" && !validRef(batch.ExpectedBaseline)) ||
		len(batch.Pages) == 0 || len(batch.Pages) > maxPages {
		return ErrInvalidBatch
	}
	return nil
}

func validRef(value string) bool {
	return refPattern.MatchString(value)
}

func (department Department) valid() bool {
	name := department.DisplayName
	return validRef(department.ExternalID) && department.SourceRevision > 0 &&
		name != "" && name == strings.TrimSpace(name) && utf8.ValidString(name) && utf8.RuneCountInString(name) <= 100 &&
		strings.IndexFunc(name, unicode.IsControl) < 0
}

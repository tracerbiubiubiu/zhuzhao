package b3protocol

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"regexp"
)

const ReportProfile = "evidence_summary_v1"

type DocumentUpdate struct {
	TypeName         string         `json:"type_name"`
	DocumentID       string         `json:"document_id"`
	Generation       string         `json:"generation"`
	ExpectedRevision string         `json:"expected_revision"`
	Patch            map[string]any `json:"patch"`
	GrantRef         string         `json:"grant_ref"`
	GrantRevision    string         `json:"grant_revision"`
}

type ReportInput struct {
	Profile     string `json:"report_profile"`
	FieldsCount int    `json:"submitted_field_count"`
	HasComment  bool   `json:"has_comment"`
}

type TaskParams struct {
	OperationID string `json:"operation_id"`
}

func Decode(reader io.Reader, value any) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	return DecodeBytes(data, value)
}

func DecodeBytes(data []byte, value any) error {
	if err := checkJSON(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
		return errors.New("unexpected JSON value after body")
	}
	return nil
}

func Identity(prefix, scope string) string {
	digest := sha256.Sum256([]byte(scope))
	return prefix + hex.EncodeToString(digest[:])
}

func TaskID(operationID string) string {
	return "b3_" + Identity("", operationID)[:40]
}

var identifier = regexp.MustCompile("^[A-Za-z0-9][A-Za-z0-9_.:-]{15,127}$")

func ValidID(value string) bool { return identifier.MatchString(value) }

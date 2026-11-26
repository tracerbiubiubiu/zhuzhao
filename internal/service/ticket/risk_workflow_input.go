package ticket

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// RiskWorkflowTicketInput carries the expected immutable pin and the common
// transition payload. It grants no authority. The owning action must still
// compare the pin and validate this patch against the actual published edge.
type RiskWorkflowTicketInput struct {
	DefinitionVersion int64
	CustomDataPatch   json.RawMessage
	Comment           string
}

// PrepareRiskWorkflowTicketInput returns an owned, normalized copy and the
// kernel TicketBinding. Use that same copy for execution, including on replay.
func PrepareRiskWorkflowTicketInput(in RiskWorkflowTicketInput) (RiskWorkflowTicketInput, string, error) {
	if in.DefinitionVersion <= 0 {
		return RiskWorkflowTicketInput{}, "", errcode.ErrInvalidParams
	}
	patch, err := normalizeTransitionPayload(in.CustomDataPatch, in.Comment)
	if err != nil {
		return RiskWorkflowTicketInput{}, "", err
	}
	in.CustomDataPatch = patch
	encoded, err := json.Marshal(struct {
		Contract          string          `json:"contract"`
		DefinitionVersion int64           `json:"definition_version"`
		Patch             json.RawMessage `json:"custom_data_patch"`
		Comment           string          `json:"comment"`
	}{"risk.ticket.transition.v1", in.DefinitionVersion, patch, in.Comment})
	if err != nil {
		return RiskWorkflowTicketInput{}, "", errcode.ErrInvalidParams
	}
	hash := sha256.Sum256(encoded)
	return in, hex.EncodeToString(hash[:]), nil
}

func normalizeTransitionPayload(raw json.RawMessage, comment string) (json.RawMessage, error) {
	if len(comment) > RichtextMaxBytes || len(raw) > CustomDataMaxBytes {
		return nil, errcode.ErrInvalidParams
	}
	patch := map[string]any{}
	if len(raw) > 0 {
		dec := json.NewDecoder(bytes.NewReader(raw))
		dec.UseNumber()
		if !json.Valid(raw) || dec.Decode(&patch) != nil || patch == nil {
			return nil, errcode.New(errcode.ErrInvalidParams.Code, "custom_data_patch 须为 JSON 对象")
		}
	}
	normal, err := json.Marshal(patch)
	if err != nil {
		return nil, errcode.ErrInvalidParams
	}
	return normal, nil
}

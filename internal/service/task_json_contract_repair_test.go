package service

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestTaskJSONContractRepair_CanonicalFields(t *testing.T) {
	for _, raw := range []string{
		`{"action_id":"audit_archive","ACTION_ID":""}`,
		`{"ACTION_ID":"audit_archive"}`,
		`{"params":{},"PARAMS":{"retention_days":1}}`,
		`{"enabled":false,"ENABLED":true}`,
		`{"CREATED_BY":"other-user"}`,
		`{"OWNER_SERVICE":"other-service"}`,
		`{"PARAMſ":{}}`,
	} {
		t.Run(raw, func(t *testing.T) {
			_, err := objectFields(json.RawMessage(raw))
			require.Error(t, err, "schema fields must have one canonical spelling before any decode or remarshal")
		})
	}
	_, err := objectFields(json.RawMessage(`{"action_id":"ordinary","params":{},"enabled":false}`))
	require.NoError(t, err)
}

func TestTaskJSONContractRepair_ArchiveParamsCanonicalFields(t *testing.T) {
	for _, raw := range []string{`{"retention_days":1,"RETENTION_DAYS":180}`, `{"RETENTION_DAYS":0}`, `{"APPROVAL_ID":"reviewed"}`} {
		t.Run(raw, func(t *testing.T) { _, err := decodeArchiveParams([]byte(raw)); require.Error(t, err) })
	}
	_, err := decodeArchiveParams([]byte(`{"retention_days":0}`))
	require.NoError(t, err)
}

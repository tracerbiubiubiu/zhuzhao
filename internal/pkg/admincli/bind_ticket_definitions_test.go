package admincli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestReadBindTicketDefinitionManifest(t *testing.T) {
	digest := strings.Repeat("a", 64)
	valid := `{
		"ticket_id":"100","type_code":"synthetic_old","original_created_by":"12",
		"original_created_at":"2026-10-01T12:34:56.123456+08:00","expected_status":"open",
		"definition_version":"1","definition_hash":"HASH","configuration_evidence_sha256":"HASH",
		"mapping_evidence_sha256":"HASH","approval_sha256":"HASH","ticket_evidence_sha256":"HASH"
	}`
	valid = strings.ReplaceAll(valid, "HASH", digest)
	manifest, err := readBindTicketDefinitionManifest(strings.NewReader(valid))
	require.NoError(t, err)
	require.Equal(t, int64(100), manifest.TicketID)
	require.Equal(t, "2026-10-01T04:34:56.123456Z", manifest.OriginalCreatedAt.Format("2006-01-02T15:04:05.999999999Z07:00"))

	for _, tc := range []struct{ name, input string }{
		{"empty", ""},
		{"null", "null"},
		{"multiple tickets", `[` + valid + `]`},
		{"second document", valid + valid},
		{"duplicate ticket", strings.Replace(valid, `"ticket_id":"100"`, `"ticket_id":"100","ticket_id":"101"`, 1)},
		{"unknown data", strings.Replace(valid, `"ticket_id":"100"`, `"ticket_id":"100","source_path":"PRIVATE"`, 1)},
		{"misnamed field", strings.Replace(valid, "ticket_id", "TICKET_ID", 1)},
		{"numeric id", strings.Replace(valid, `"100"`, `100`, 1)},
		{"noncanonical id", strings.Replace(valid, `"100"`, `"0100"`, 1)},
		{"zero definition version", strings.Replace(valid, `"1"`, `"0"`, 1)},
		{"incomplete source", strings.Replace(valid, `"approval_sha256":"`+digest+`"`, `"approval_sha256":""`, 1)},
		{"uppercase digest", strings.Replace(valid, digest, strings.ToUpper(digest), 1)},
		{"timestamp rounding", strings.Replace(valid, ".123456+", ".1234567+", 1)},
		{"oversize", valid + strings.Repeat(" ", 8192)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readBindTicketDefinitionManifest(strings.NewReader(tc.input))
			require.Error(t, err)
			require.NotContains(t, err.Error(), "PRIVATE")
		})
	}
}

func TestCheckLegacyDefinitionSnapshot(t *testing.T) {
	const snapshot = `{"type":{"code":"example","states":["open","assigned","closed"],
		"transitions":{"open":["closed"],"closed":["open"]},"transition_meta":{},
		"default_sla_hours":24,"has_custom_fields":true},"fields":[{"field_id":"9","type_code":"example",
		"field_key":"plan","field_label":"Plan","field_type":"input","required":true,"sort_order":10}]}`
	require.NoError(t, checkLegacyDefinitionSnapshot([]byte(snapshot), "example", "open"))
	require.NoError(t, checkLegacyDefinitionSnapshot([]byte(strings.Replace(snapshot, `"transition_meta":{}`, `"transition_meta":null`, 1)), "example", "open"))
	for _, tc := range []struct{ name, value string }{
		{"missing SLA", strings.Replace(snapshot, `"default_sla_hours":24,`, "", 1)},
		{"missing transition metadata", strings.Replace(snapshot, `"transition_meta":{},`, "", 1)},
		{"null field", snapshot[:strings.Index(snapshot, `[{"field_id"`)] + `[null]}`},
		{"missing requirement", strings.Replace(snapshot, `"required":true,`, "", 1)},
		{"wrong field type code", strings.Replace(snapshot, `"type_code":"example"`, `"type_code":"unrelated"`, 1)},
		{"missing assigned", strings.Replace(snapshot, `"assigned",`, "", 1)},
		{"unsupported field type", strings.Replace(snapshot, `"field_type":"input"`, `"field_type":"custom"`, 1)},
		{"orphan metadata edge", strings.Replace(snapshot, `"transition_meta":{}`, `"transition_meta":{"assigned>closed":{}}`, 1)},
		{"invalid metadata payload", strings.Replace(snapshot, `"transition_meta":{}`, `"transition_meta":{"open>closed":[]}`, 1)},
		{"invalid target on another edge", strings.Replace(snapshot, `"closed":["open"]`, `"closed":["missing"]`, 1)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Error(t, checkLegacyDefinitionSnapshot([]byte(tc.value), "example", "open"))
		})
	}
}

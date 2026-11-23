package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/docs"
)

func TestRiskDispatchAndRemediationOpenAPI(t *testing.T) {
	type schemaProperty struct {
		Type                 string
		Ref                  string `json:"$ref"`
		AdditionalProperties json.RawMessage
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Parameters []struct {
				In       string
				Required bool
				Schema   struct {
					Ref string `json:"$ref"`
				}
			}
		}
		Definitions map[string]struct {
			schemaProperty
			Properties map[string]schemaProperty
			Required   []string
		}
	}
	require.NoError(t, json.Unmarshal([]byte(docs.SwaggerInfo.ReadDoc()), &spec))
	require.ElementsMatch(t, []string{"operation_id", "ticket_id", "expected_ticket_revision", "definition_version", "items"},
		spec.Definitions["internal_handler.riskWorkflowSubmitRequest"].Required)
	for _, path := range []string{"dispatch-plan", "dispatch", "batch-dispatch"} {
		t.Run(path, func(t *testing.T) {
			methods := spec.Paths["/api/v1/risk-items/"+path]
			require.Contains(t, methods, "post")
			body := methods["post"].Parameters
			require.Len(t, body, 1)
			require.Equal(t, "body", body[0].In)
			require.True(t, body[0].Required)
			require.NotEmpty(t, body[0].Schema.Ref)
		})
	}
	for name, schema := range spec.Definitions {
		if patch, ok := schema.Properties["custom_data_patch"]; ok && strings.Contains(name, "riskWorkflow") {
			if patch.Ref != "" {
				patch = spec.Definitions[strings.TrimPrefix(patch.Ref, "#/definitions/")].schemaProperty
			}
			require.Equal(t, "object", patch.Type, "%s.custom_data_patch", name)
			require.NotEmpty(t, patch.AdditionalProperties, "%s accepts configured field names", name)
			require.NotEqual(t, "false", string(patch.AdditionalProperties))
		}
	}
}

func TestRiskWorkflowRequestPreservesCustomFieldNumbers(t *testing.T) {
	for _, tc := range []struct {
		name, patch, want string
	}{
		{name: "omitted"},
		{name: "object", patch: `,"custom_data_patch":{"example":{"number":9007199254740993}}`, want: `{"example":{"number":9007199254740993}}`},
		{name: "null", patch: `,"custom_data_patch":null`},
		{name: "array", patch: `,"custom_data_patch":[]`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(rec)
			c.Request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(
				`{"operation_id":"d1f72e3f-5810-4f40-87d2-876873895c37","ticket_id":"1","expected_ticket_revision":"1","definition_version":"1"`+tc.patch+`}`))
			req, ok := riskWorkflowInput(c)
			if tc.patch != "" && tc.want == "" {
				require.False(t, ok)
				require.Equal(t, http.StatusBadRequest, rec.Code)
				return
			}
			require.True(t, ok, rec.Body.String())
			_, input := req.inputs(1)
			require.Equal(t, tc.want, string(input.CustomDataPatch))
		})
	}
}

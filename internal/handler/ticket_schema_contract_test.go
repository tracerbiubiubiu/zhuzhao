package handler

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestTicketTypeFieldContractIncludesAllDeliveredTypes(t *testing.T) {
	want := []string{"input", "textarea", "number", "date", "select", "multi_select", "tips", "richtext", "radio", "rating", "file"}
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "swagger.json"))
	require.NoError(t, err)
	var document struct {
		Definitions map[string]struct {
			Properties map[string]struct {
				Enum []any `json:"enum"`
			} `json:"properties"`
		} `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	var enums []any
	for name, definition := range document.Definitions {
		if strings.HasSuffix(name, ".TicketTypeFieldInput") {
			require.Nil(t, enums, "field definition must be unambiguous")
			enums = definition.Properties["field_type"].Enum
		}
	}
	require.ElementsMatch(t, want, enums, "generated clients must be able to express every delivered field type")
}

func TestTicketTypeFieldJSONBindingAcceptsDeliveredTypes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	for _, fieldType := range []string{"input", "textarea", "number", "date", "select", "multi_select", "tips", "richtext", "radio", "rating", "file"} {
		t.Run(fieldType, func(t *testing.T) {
			body, err := json.Marshal(model.ReplaceTypeFieldsRequest{Code: "synthetic", Fields: []model.TicketTypeFieldInput{{FieldKey: "value", FieldLabel: "Value", FieldType: fieldType}}})
			require.NoError(t, err)
			c, _ := gin.CreateTestContext(httptest.NewRecorder())
			c.Request = httptest.NewRequest("POST", "/api/v1/ticket-types/fields/replace", bytes.NewReader(body))
			c.Request.Header.Set("Content-Type", "application/json")
			var request model.ReplaceTypeFieldsRequest
			require.NoError(t, c.ShouldBindJSON(&request))
			require.Equal(t, fieldType, request.Fields[0].FieldType)
		})
	}
}

// The public schema must describe the JSON object accepted by G2/mergeCustomData,
// rather than RawMessage's underlying byte slice. Clients generate types from it.
func TestTicketCustomDataSwaggerObject(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "docs", "swagger.json"))
	require.NoError(t, err)
	var document struct {
		Definitions map[string]struct {
			Properties map[string]struct {
				Type  string `json:"type"`
				Items any    `json:"items"`
			} `json:"properties"`
		} `json:"definitions"`
	}
	require.NoError(t, json.Unmarshal(raw, &document))
	value := json.RawMessage(`{"score":3,"tags":["one"]}`)
	cases := []struct {
		name string
		wire any
	}{
		{"CreateTicketRequest", model.CreateTicketRequest{CustomData: value}},
		{"UpdateTicketRequest", model.UpdateTicketRequest{CustomData: value}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var names []string
			for name := range document.Definitions {
				if strings.HasSuffix(name, "."+tc.name) {
					names = append(names, name)
				}
			}
			require.Len(t, names, 1, "public request definition must be unambiguous")
			property, ok := document.Definitions[names[0]].Properties["custom_data"]
			require.True(t, ok, "missing custom_data schema")
			require.Equal(t, "object", property.Type, "wire object must generate an object type")
			require.Nil(t, property.Items, "object must not retain an array items schema")
			encoded, err := json.Marshal(tc.wire)
			require.NoError(t, err)
			var decoded map[string]any
			require.NoError(t, json.Unmarshal(encoded, &decoded))
			object, ok := decoded["custom_data"].(map[string]any)
			require.True(t, ok, "actual JSON must agree with the object schema")
			require.Equal(t, float64(3), object["score"])
			require.Equal(t, []any{"one"}, object["tags"])
		})
	}
}

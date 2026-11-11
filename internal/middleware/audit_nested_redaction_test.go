package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMaskSensitive_NestedArrays(t *testing.T) {
	for name, input := range map[string]string{
		"direct map": `{"child":{"PaSsWoRd":"synthetic-secret"}}`,
		"array":      `{"rows":[{"secret":"synthetic-secret"}]}`,
		"two arrays": `{"rows":[[{"Refresh_Token":"synthetic-secret"}]]}`,
		"three arrays": `{"rows":[[[{"old_password":"synthetic-secret",` +
			`"items":[[{"new_password":"synthetic-secret","TOKEN":"synthetic-secret"}]]}]]]}`,
	} {
		t.Run(name, func(t *testing.T) {
			out := maskSensitive([]byte(input))
			require.True(t, json.Valid([]byte(out)), "audit body remains JSON")
			assert.NotContains(t, out, "synthetic-secret")
			assert.Contains(t, out, `"***"`)
		})
	}

	const safe = `{"rows":[[null,42,true,"text",{"tokens":"keep","description":"keep"}],[]]}`
	assert.JSONEq(t, safe, maskSensitive([]byte(safe)), "non-sensitive JSON keeps its shape and values")
}

type nestedRedactionAuditCapture struct{ entry AuditLogEntry }

func (a *nestedRedactionAuditCapture) Insert(_ context.Context, entry AuditLogEntry) error {
	a.entry = entry
	return nil
}

func TestAuditLog_NestedArrayRedaction(t *testing.T) {
	gin.SetMode(gin.TestMode)
	sink := &nestedRedactionAuditCapture{}
	router := gin.New()
	router.Use(AuditLog(sink))
	router.POST("/probe", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"rows": []any{[]any{gin.H{
			"refresh_token": "synthetic-response-token", "safe": "kept",
		}}}}})
	})
	body := `{"params":{"rows":[[{"PaSsWoRd":"synthetic-request-password","safe":"kept"}]]},"password":"direct"}`
	request := httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	require.Equal(t, http.StatusOK, response.Code)
	assert.Contains(t, sink.entry.RequestBody, `"password":"***"`, "direct-mask positive control")
	assert.NotContains(t, sink.entry.RequestBody, "synthetic-request-password")
	assert.NotContains(t, sink.entry.ResponseSummary, "synthetic-response-token")
	assert.Contains(t, sink.entry.RequestBody, "kept")
	assert.Contains(t, sink.entry.ResponseSummary, "kept")
	assert.Contains(t, response.Body.String(), "synthetic-response-token", "response capture does not mutate the response")
}

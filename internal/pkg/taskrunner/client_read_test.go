package taskrunner

import (
	"context"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRawResponseAvailableAfterDecode(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":0,"data":{"job_id":"job","action_id":"ordinary","task_id":"task"}}`))
	}))
	defer server.Close()
	c := New(Config{BaseURL: server.URL, SK: []byte("synthetic")})
	for _, read := range []func() (string, error){func() (string, error) { v, e := c.GetJob(context.Background(), "job"); return string(v), e }, func() (string, error) { v, e := c.GetTask(context.Background(), "task"); return string(v), e }} {
		got, err := read()
		require.NoError(t, err)
		require.Contains(t, got, `"job_id":"job"`)
	}
}

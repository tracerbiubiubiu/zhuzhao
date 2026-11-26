package referencequery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

func requestFixture() QueryRequest {
	return QueryRequest{QueryID: QueryID, ContractVersion: Version, SourceID: "ref-assets-v1", Parameters: Parameters{OwnerOrgRefs: []string{"91001"}}, Limit: 2, RequestedFields: []string{"device_id", "owner_org_ref", "stale", "observed_at"}}
}

func pageFixture() QueryPage {
	return QueryPage{Items: []map[string]any{{"device_id": "1", "owner_org_ref": "91001", "stale": false, "observed_at": nil}}, Complete: true, SourceID: "ref-assets-v1", ObservedAt: "2026-01-01T00:00:00Z", FetchedAt: "2026-01-01T00:00:01Z", Warnings: []string{}, MappingVersion: "v1"}
}

func TestScopeAndFields(t *testing.T) {
	q := requestFixture()
	require.NoError(t, ValidateRequest(q))
	q.Parameters.OwnerOrgRefs = nil
	require.NoError(t, ValidateInput(q))
	require.Error(t, ValidateRequest(q))
	for _, change := range []func(*QueryRequest){
		func(q *QueryRequest) { q.Parameters.OwnerOrgRefs = []string{"01"} },
		func(q *QueryRequest) { q.Parameters.OwnerOrgRefs = []string{"1", "1"} },
		func(q *QueryRequest) { q.Limit = 51 },
		func(q *QueryRequest) { q.RequestedFields = []string{"secret"} },
		func(q *QueryRequest) { q.Parameters.Status = []string{"active"} },
	} {
		q := requestFixture()
		change(&q)
		require.Error(t, ValidateRequest(q))
	}
	for _, change := range []func(*QueryPage){
		func(p *QueryPage) { p.Items[0]["extra"] = "unrequested" },
		func(p *QueryPage) { p.Items[0]["owner_org_ref"] = "91002" },
		func(p *QueryPage) { p.Items[0]["stale"] = "true" },
		func(p *QueryPage) { p.SourceID = "other" },
		func(p *QueryPage) { p.NextCursor = "next" },
		func(p *QueryPage) { p.Items = append(p.Items, p.Items[0]) },
	} {
		p := pageFixture()
		change(&p)
		require.Error(t, ValidatePage(requestFixture(), p))
	}
	require.NoError(t, ValidatePage(requestFixture(), pageFixture()))
}

func TestTransport(t *testing.T) {
	q := requestFixture()
	key := "fixture-key"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "POST", r.Method)
		require.Equal(t, "/v1/query", r.URL.Path)
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		v := aksk.Verifier{Keys: map[string][]byte{"fixture": []byte(key)}}
		require.NoError(t, v.Verify(r, b))
		var in QueryRequest
		require.NoError(t, json.Unmarshal(b, &in))
		require.Equal(t, q, in)
		require.NoError(t, json.NewEncoder(w).Encode(pageFixture()))
	}))
	defer s.Close()
	c, err := NewClient(s.URL, "fixture", key, time.Second)
	require.NoError(t, err)
	got, err := c.Query(context.Background(), q)
	require.NoError(t, err)
	require.Equal(t, pageFixture(), got)
}

func TestRefuseUnsafeResponsesAndTargets(t *testing.T) {
	for _, origin := range []string{"http://example.invalid", "https://user@localhost", "https://localhost/path", "https://localhost?query=x"} {
		_, err := NewPeer(origin, "fixture", "fixture-key", time.Second)
		require.Error(t, err)
	}
	for _, tc := range []struct {
		status int
		body   string
		want   int
	}{
		{403, "vendor-sensitive-message", 403}, {429, "", 429}, {302, "", 503},
		{200, strings.Repeat("x", 1<<20+1), 503}, {200, "{}", 409},
	} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if tc.status == 302 {
				w.Header().Set("Location", "/never")
			}
			w.WriteHeader(tc.status)
			_, _ = io.WriteString(w, tc.body)
		}))
		c, err := NewClient(s.URL, "fixture", "fixture-key", time.Second)
		require.NoError(t, err)
		_, err = c.Query(context.Background(), requestFixture())
		s.Close()
		var f *Failure
		require.ErrorAs(t, err, &f)
		require.Equal(t, tc.want, f.Status)
		require.NotContains(t, err.Error(), "vendor-sensitive")
	}
}

func TestSignedInvocation(t *testing.T) {
	inv := Invocation{RequestID: "ref-123", Method: "GET", Path: QueryPath, Query: requestFixture()}
	v := &aksk.Verifier{Keys: map[string][]byte{"fixture": []byte("key")}}
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var decoded Invocation
		if f := ReadSigned(r, v, AdmissionPath, &decoded); f != nil {
			WriteFailure(w, f)
			return
		}
		require.Equal(t, inv, decoded)
		require.NoError(t, json.NewEncoder(w).Encode(Admission{Query: requestFixture()}))
	}))
	defer s.Close()
	p, err := NewPeer(s.URL, "fixture", "key", time.Second)
	require.NoError(t, err)
	var granted Admission
	require.NoError(t, p.Post(context.Background(), AdmissionPath, &inv, &granted))
	require.Equal(t, requestFixture(), granted.Query)
	require.Error(t, p.Post(context.Background(), "/other", inv, &granted))
	for _, body := range []string{`{"request_id":"other"}`, `{"request_id":"ref-123","user_id":"1"}`, `{"request_id":"ref-123","request_id":"ref-123"}`} {
		r := httptest.NewRequest("POST", AdmissionPath, strings.NewReader(body))
		aksk.Sign(r, []byte(body), aksk.SignOptions{AK: "fixture", SK: []byte("key"), RequestID: "ref-123"})
		require.NotNil(t, ReadSigned(r, v, AdmissionPath, &Invocation{}))
	}
}

func TestPeerDeadlineAndPublicError(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		<-r.Context().Done()
	}))
	defer s.Close()
	client, err := NewClient(s.URL, "fixture", "key", 25*time.Millisecond)
	require.NoError(t, err)
	_, err = client.Query(context.Background(), requestFixture())
	var f *Failure
	require.ErrorAs(t, err, &f)
	require.Equal(t, 503, f.Status)

	s2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"reason":"quota","retry_after_ms":12}`)
	}))
	defer s2.Close()
	client, err = NewClient(s2.URL, "fixture", "key", time.Second)
	require.NoError(t, err)
	_, err = client.Query(context.Background(), requestFixture())
	require.ErrorAs(t, err, &f)
	require.Equal(t, 429, f.Status)
	require.Equal(t, 12, *f.RetryAfterMS)
}

func TestNarrowScopeAndStrictEnvelopes(t *testing.T) {
	q := requestFixture()
	q.Parameters.Status = []string{"healthy"}
	q.RequestedFields = append(q.RequestedFields, "status")
	p := pageFixture()
	p.Items[0]["status"] = "degraded"
	require.Error(t, ValidatePage(q, p))
	p.Items[0]["status"] = "healthy"
	p.Complete = false
	p.Warnings = []string{"source_stale"}
	require.NoError(t, ValidatePage(q, p))
	raw, err := json.Marshal(requestFixture())
	require.NoError(t, err)
	for _, pair := range [][2]string{
		{`"parameters":{`, `"parameters":{"status":null,`},
		{`"parameters":`, `"cursor":null,"parameters":`},
		{`"parameters":`, `"cursor":"","parameters":`},
	} {
		body := strings.Replace(string(raw), pair[0], pair[1], 1)
		r := httptest.NewRequest("POST", ConnectorPath, strings.NewReader(body))
		aksk.Sign(r, []byte(body), aksk.SignOptions{AK: "fixture", SK: []byte("key")})
		require.NotNil(t, ReadSigned(r, &aksk.Verifier{Keys: map[string][]byte{"fixture": []byte("key")}}, ConnectorPath, &QueryRequest{}))
	}
}

package b3al

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

func example() Command {
	now := time.Now().UTC()
	return Command{OperationID: "op-demo-0000000001", InvocationID: "inv-demo-000000001", SubjectType: "human", SubjectID: "2", AuthorizationRef: "auth-demo-000000001", AuthorizationRevision: "7", TypeName: "device_status", DocumentID: "1001", Generation: "7", ExpectedRevision: "42", Fence: "7", RootID: "root-demo-000000001", RootBudgetRef: "budget-demo-0000001", Entrypoint: "schedule", Patch: map[string]any{"status": "healthy"}, SubmittedAt: now, ExpiresAt: now.Add(time.Minute)}
}

func TestUpdateUsesPublishedWireVector(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/internal/activelist/documents/update" || r.URL.RawQuery != "" || r.Header.Get("X-Operator") != "" {
			t.Error("wrong transport", r.URL)
		}
		var req struct {
			Kind    string
			Command struct {
				Hash string `json:"canonical_input_hash"`
			}
			Admission struct {
				Hash      string `json:"canonical_input_hash"`
				Purpose   string
				IssuedAt  time.Time `json:"issued_at"`
				ExpiresAt time.Time `json:"expires_at"`
			}
		}
		raw, err := io.ReadAll(r.Body)
		verifier := aksk.Verifier{Keys: map[string][]byte{"synthetic": []byte("synthetic-key")}}
		if err != nil || verifier.Verify(r, raw) != nil {
			t.Error("body signature did not verify")
		}
		if err := json.Unmarshal(raw, &req); err != nil {
			t.Error(err)
		}
		if req.Kind != "al_execution_request" || req.Command.Hash != "a87db27127eee5cb2b638224ec1055108458d31e053d469874095b365baadc19" || req.Admission.Hash != req.Command.Hash || req.Admission.Purpose != "al_document_update" || req.Admission.ExpiresAt.Sub(req.Admission.IssuedAt) != 30*time.Second {
			t.Error(req)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"type_name":"device_status","document_id":"1001","generation":"7","document_revision":"43","noop":false}}`))
	}))
	defer srv.Close()
	c, err := NewClient(srv.URL, "synthetic", "synthetic-key")
	if err != nil {
		t.Fatal(err)
	}
	got, err := c.Update(context.Background(), example())
	if err != nil || got.DocumentRevision != "43" {
		t.Fatal(got, err)
	}
}

func TestAuthorizationBindsGrantAndFields(t *testing.T) {
	cmd := example()
	grant := Grant{SubjectType: "human", SubjectID: "2", AuthorizationRef: cmd.AuthorizationRef, AuthorizationRevision: "7", Generation: "7", DocumentID: "1001", TypeName: "device_status", WriteFields: []string{"status"}}
	if err := Authorize(cmd, grant, true); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func(*Command){func(c *Command) { c.DocumentID = "1002" }, func(c *Command) { c.AuthorizationRevision = "6" }, func(c *Command) { c.Patch = map[string]any{"owner": "other"} }, func(c *Command) { c.SubjectID = "3" }} {
		x := cmd
		mutate(&x)
		if Authorize(x, grant, true) == nil {
			t.Fatal("scope mismatch permitted")
		}
	}
}

func TestGetRefusesUnboundOrExcessiveProjection(t *testing.T) {
	cmd := example()
	cmd.SubjectType = "service"
	cmd.Patch = nil
	cmd.ExpectedRevision = ""
	cmd.Fields = []string{"status"}
	for _, body := range []string{
		`{"code":0,"data":{"type_name":"device_status","document_id":"1002","generation":"7","document_revision":"43","queried_at":"2026-01-01T00:00:00Z","values":{}}}`,
		`{"code":0,"data":{"type_name":"device_status","document_id":"1001","generation":"7","document_revision":"43","queried_at":"2026-01-01T00:00:00Z","values":{"hidden":"secret"}}}`,
	} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(body)) }))
		c, err := NewClient(srv.URL, "synthetic", "synthetic-key")
		if err != nil {
			t.Fatal(err)
		}
		_, err = c.Get(context.Background(), cmd)
		srv.Close()
		if err == nil {
			t.Fatal("unbound result accepted")
		}
	}
}

func TestGetRequiresOriginalEventRevision(t *testing.T) {
	cmd := example()
	cmd.SubjectType = "service"
	cmd.Patch = nil
	cmd.ExpectedRevision = ""
	cmd.Fields = []string{"status"}
	cmd.SourceEventID = "al-event-00000000001"
	if _, err := cmd.envelope(time.Now(), getPath); err == nil {
		t.Fatal("unbound observation admitted")
	}
}

func TestGetEmptyProjectionBoundToOriginalEvent(t *testing.T) {
	cmd := example()
	cmd.SubjectType = "service"
	cmd.ExpectedRevision = ""
	cmd.Patch = nil
	cmd.Fields = []string{"status"}
	cmd.SourceEventID = "al-event-00000000001"
	cmd.SourceEventRevision = "42"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != getPath {
			t.Error(r.URL.Path)
		}
		var req map[string]any
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
		}
		if req["admission"].(map[string]any)["purpose"] != "al_document_get" {
			t.Error("wrong purpose")
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"type_name":"device_status","document_id":"1001","generation":"7","document_revision":"44","queried_at":"2026-01-01T00:00:00Z","values":{}}}`))
	}))
	defer srv.Close()
	cl, err := NewClient(srv.URL, "synthetic", "key")
	if err != nil {
		t.Fatal(err)
	}
	result, err := cl.Get(context.Background(), cmd)
	if err != nil || len(result.Values) != 0 || result.SourceEventRevision != "42" || result.DocumentRevision != "44" || result.ObservationOwner != "zhuzhao" {
		t.Fatal(result, err)
	}
}

func TestReceiptLookupUsesFreshReadOnlyAdmissionAfterDeadline(t *testing.T) {
	cmd := example()
	cmd.SubmittedAt = time.Now().Add(-2 * time.Hour)
	cmd.ExpiresAt = cmd.SubmittedAt.Add(time.Hour)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/activelist/documents/receipt" {
			t.Error(r.URL.Path)
		}
		var b struct {
			Kind      string
			Admission struct {
				Purpose   string
				ExpiresAt time.Time `json:"expires_at"`
			}
			Command struct {
				ExpiresAt time.Time `json:"expires_at"`
				Hash      string    `json:"canonical_input_hash"`
			}
		}
		if err := json.NewDecoder(r.Body).Decode(&b); err != nil {
			t.Error(err)
		}
		if b.Kind != "al_receipt_request" || b.Admission.Purpose != "al_effect_receipt" || !b.Admission.ExpiresAt.After(time.Now()) || !b.Command.ExpiresAt.Equal(cmd.ExpiresAt) || b.Command.Hash != "a87db27127eee5cb2b638224ec1055108458d31e053d469874095b365baadc19" {
			t.Error("bad reconciliation binding", b)
		}
		_, _ = w.Write([]byte(`{"code":0,"data":{"type_name":"device_status","document_id":"1001","generation":"7","document_revision":"43","noop":false}}`))
	}))
	defer srv.Close()
	cl, err := NewClient(srv.URL, "synthetic", "synthetic-key")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = cl.Update(context.Background(), cmd); err == nil {
		t.Fatal("expired command executed")
	}
	got, err := cl.LookupUpdate(context.Background(), cmd)
	if err != nil || got.DocumentRevision != "43" {
		t.Fatal(got, err)
	}
}

func TestProfileBindsActionToSubjectKind(t *testing.T) {
	write := example()
	write.SubjectType = "service"
	if _, err := write.envelope(time.Now(), updatePath); err == nil {
		t.Fatal("service write allowed in human-only profile")
	}
	read := example()
	read.ExpectedRevision = ""
	read.Patch = nil
	read.Fields = []string{"status"}
	if _, err := read.envelope(time.Now(), getPath); err == nil {
		t.Fatal("human used subscription read profile")
	}
}

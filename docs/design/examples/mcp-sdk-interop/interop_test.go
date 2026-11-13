package interop

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/jsonschema-go/jsonschema"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

const revision = "2026-07-28"

func syntheticSubject(s string) bool { return len(s) == 6 && s >= "demo-a" && s <= "demo-e" }

type tokenRecord struct {
	Subject, Audience, Issuer, Client, Scope string
	Expiry                                   int64
	Active                                   bool
}
type codeRecord struct {
	Subject, Audience, Client, Redirect, Challenge string
	Used                                           bool
}
type testAS struct {
	mu        sync.Mutex
	server    *httptest.Server
	codes     map[string]*codeRecord
	tokens    map[string]*tokenRecord
	down      bool
	sequence  int
	exchanges int
}

func newAS(t *testing.T) *testAS {
	a := &testAS{codes: map[string]*codeRecord{}, tokens: map[string]*tokenRecord{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-authorization-server", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"issuer": a.server.URL, "authorization_endpoint": a.server.URL + "/authorize", "token_endpoint": a.server.URL + "/token", "code_challenge_methods_supported": []string{"S256"}, "response_types_supported": []string{"code"}})
	})
	mux.HandleFunc("/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		client := q.Get("client_id")
		redirect := "http://127.0.0.1/callback/" + client
		if (client != "client-a" && client != "client-b") || q.Get("redirect_uri") != redirect || q.Get("code_challenge_method") != "S256" || q.Get("code_challenge") == "" || q.Get("state") == "" || q.Get("resource") == "" {
			http.Error(w, "invalid authorization request", 400)
			return
		}
		subject := r.Header.Get("X-Demo-Authenticated-Subject")
		if !syntheticSubject(subject) {
			http.Error(w, "fixture login required", 401)
			return
		}
		a.mu.Lock()
		a.sequence++
		code := fmt.Sprint("authorization-code-", a.sequence)
		a.codes[code] = &codeRecord{Subject: subject, Audience: q.Get("resource"), Client: client, Redirect: redirect, Challenge: q.Get("code_challenge")}
		a.mu.Unlock()
		callback, _ := url.Parse(redirect)
		v := callback.Query()
		v.Set("code", code)
		v.Set("state", q.Get("state"))
		callback.RawQuery = v.Encode()
		http.Redirect(w, r, callback.String(), 302)
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		a.mu.Lock()
		defer a.mu.Unlock()
		c := a.codes[r.Form.Get("code")]
		h := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		challenge := base64.RawURLEncoding.EncodeToString(h[:])
		if c == nil || c.Used || c.Client != r.Form.Get("client_id") || c.Redirect != r.Form.Get("redirect_uri") || c.Audience != r.Form.Get("resource") || challenge != c.Challenge {
			writeStatusJSON(w, 400, map[string]any{"error": "invalid_grant"})
			return
		}
		c.Used = true
		a.exchanges++
		token := fmt.Sprint("fixture-opaque-", a.exchanges)
		scope := "ticket:list"
		if strings.Contains(c.Audience, "external") {
			scope = "intel:query"
		}
		a.tokens[token] = &tokenRecord{Subject: c.Subject, Audience: c.Audience, Issuer: a.server.URL, Client: c.Client, Scope: scope, Expiry: time.Now().Add(time.Hour).Unix(), Active: true}
		writeJSON(w, map[string]any{"access_token": token, "token_type": "Bearer", "expires_in": 3600, "scope": scope})
	})
	mux.HandleFunc("/introspect", func(w http.ResponseWriter, r *http.Request) {
		user, password, ok := r.BasicAuth()
		if !ok || user != "resource-server-fixture" || password != "synthetic-only" {
			http.Error(w, "unauthorized introspection client", 401)
			return
		}
		_ = r.ParseForm()
		a.mu.Lock()
		defer a.mu.Unlock()
		if a.down {
			http.Error(w, "fixture AS unavailable", 503)
			return
		}
		x := a.tokens[r.Form.Get("token")]
		if x == nil {
			writeJSON(w, map[string]any{"active": false})
			return
		}
		writeJSON(w, map[string]any{"active": x.Active, "sub": x.Subject, "aud": x.Audience, "iss": x.Issuer, "client_id": x.Client, "scope": x.Scope, "exp": x.Expiry})
	})
	a.server = httptest.NewServer(mux)
	t.Cleanup(a.server.Close)
	return a
}
func writeJSON(w http.ResponseWriter, x any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(x)
}
func writeStatusJSON(w http.ResponseWriter, status int, x any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(x)
}

type authFlow struct {
	config                            oauth2.Config
	state, verifier, audience, client string
	used                              bool
}

func (a *testAS) begin(t *testing.T, subject, client, audience string) (*authFlow, *url.URL) {
	t.Helper()
	flow := &authFlow{state: "state-bound-to-" + client + "-" + subject, verifier: strings.Repeat("v", 43), audience: audience, client: client}
	flow.config = oauth2.Config{ClientID: client, ClientSecret: "synthetic-" + client, RedirectURL: "http://127.0.0.1/callback/" + client, Endpoint: oauth2.Endpoint{AuthURL: a.server.URL + "/authorize", TokenURL: a.server.URL + "/token", AuthStyle: oauth2.AuthStyleInParams}}
	target := flow.config.AuthCodeURL(flow.state, oauth2.S256ChallengeOption(flow.verifier), oauth2.SetAuthURLParam("resource", audience))
	req, _ := http.NewRequest("GET", target, nil)
	req.Header.Set("X-Demo-Authenticated-Subject", subject)
	clientHTTP := &http.Client{Timeout: 2 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, err := clientHTTP.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 302 {
		t.Fatalf("authorize status %d", response.StatusCode)
	}
	callback, err := url.Parse(response.Header.Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	return flow, callback
}
func (f *authFlow) finish(callback *url.URL) (*oauth2.Token, error) {
	if f.used || callback.Query().Get("state") != f.state || callback.Scheme+"://"+callback.Host+callback.Path != f.config.RedirectURL {
		return nil, errors.New("callback state/client/redirect invalid or replayed")
	}
	f.used = true
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return f.config.Exchange(ctx, callback.Query().Get("code"), oauth2.VerifierOption(f.verifier), oauth2.SetAuthURLParam("resource", f.audience))
}
func (a *testAS) issue(t *testing.T, subject, client, audience string) *oauth2.Token {
	t.Helper()
	f, cb := a.begin(t, subject, client, audience)
	tok, err := f.finish(cb)
	if err != nil {
		t.Fatal(err)
	}
	return tok
}

type system struct {
	as            *testAS
	server        *httptest.Server
	external      bool
	mu            sync.Mutex
	local         map[string]bool
	inflight      map[string]int
	totalInflight int
	entered       chan struct{}
	coreDown      bool
	totalOverflow bool
	oversized     bool
	remoteDenied  bool
	drift         bool
	malicious     string
	block         chan struct{}
	cancelled     chan struct{}
	calls         atomic.Int64
	reads         atomic.Int64
	methods       map[string]int
	observedCaps  []map[string]any
	contentTypes  []string
	bundle        map[string]map[string]any
	schemas       map[string]*jsonschema.Resolved
}

func loadTools(t *testing.T) (map[string]map[string]any, map[string]*jsonschema.Resolved) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "mcp-first-contract.schema.json"))
	if err != nil {
		t.Fatal(err)
	}
	var bundle struct{ Tools []map[string]any }
	if err = json.Unmarshal(b, &bundle); err != nil {
		t.Fatal(err)
	}
	tools := map[string]map[string]any{}
	resolved := map[string]*jsonschema.Resolved{}
	for _, tool := range bundle.Tools {
		name := tool["name"].(string)
		tools[name] = tool
		for _, field := range []string{"inputSchema", "outputSchema"} {
			raw, _ := json.Marshal(tool[field])
			var schema jsonschema.Schema
			if err = json.Unmarshal(raw, &schema); err != nil {
				t.Fatal(err)
			}
			r, e := schema.Resolve(nil)
			if e != nil {
				t.Fatalf("%s %s schema: %v", name, field, e)
			}
			resolved[name+"/"+field] = r
		}
	}
	return tools, resolved
}
func newSystem(t *testing.T, a *testAS, external, jsonResponse bool) *system {
	t.Helper()
	tools, schemas := loadTools(t)
	h := &system{as: a, external: external, local: map[string]bool{"demo-a": true, "demo-b": true, "demo-c": true, "demo-d": true, "demo-e": true}, inflight: map[string]int{}, entered: make(chan struct{}, 16), methods: map[string]int{}, bundle: tools, schemas: schemas, cancelled: make(chan struct{}, 1)}
	server := mcp.NewServer(&mcp.Implementation{Name: "synthetic-zhuzhao-reference", Version: "1"}, &mcp.ServerOptions{SupportedProtocolVersions: []string{revision}})
	names := []string{"zhuzhao_ticket_list_v1", "zhuzhao_ticket_get_v1"}
	if external {
		names = []string{"mock_intel_lookup_v1"}
	}
	for _, name := range names {
		name := name
		tool := tools[name]
		server.AddTool(&mcp.Tool{Name: name, Description: "synthetic design-contract fixture", InputSchema: tool["inputSchema"], OutputSchema: tool["outputSchema"]}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
			return h.handle(ctx, req, name)
		})
	}
	server.AddReceivingMiddleware(func(next mcp.MethodHandler) mcp.MethodHandler {
		return func(ctx context.Context, method string, req mcp.Request) (mcp.Result, error) {
			extra := req.GetExtra()
			subject := ""
			if extra != nil && extra.TokenInfo != nil {
				subject = extra.TokenInfo.UserID
			}
			h.mu.Lock()
			h.methods[method]++
			local := h.local[subject]
			down := h.coreDown
			drift := h.drift
			h.mu.Unlock()
			if method == "tools/list" && !local {
				return &mcp.ListToolsResult{Tools: []*mcp.Tool{}}, nil
			}
			if method == "tools/call" && down {
				return toolError(92601, "authorization unavailable"), nil
			}
			if method == "tools/call" && !local {
				return toolError(70001, "permission denied"), nil
			}
			result, err := next(ctx, method, req)
			if method == "tools/list" && drift && err == nil {
				if list, ok := result.(*mcp.ListToolsResult); ok && len(list.Tools) > 0 {
					list.Tools[0].InputSchema = map[string]any{"type": "object", "properties": map[string]any{"different": map[string]any{"type": "string"}}}
				}
			}
			return result, err
		}
	})
	stream := mcp.NewStreamableHTTPHandler(func(r *http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: jsonResponse, MaxRequestBodyBytes: 64 << 10, PropagateRequestCancellation: true})
	scope := "ticket:list"
	if external {
		scope = "intel:query"
	}
	verify := func(ctx context.Context, token string, r *http.Request) (*auth.TokenInfo, error) {
		form := url.Values{"token": []string{token}}
		req, _ := http.NewRequestWithContext(ctx, "POST", a.server.URL+"/introspect", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.SetBasicAuth("resource-server-fixture", "synthetic-only")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, errors.New("AS unavailable")
		}
		defer res.Body.Close()
		if res.StatusCode != 200 {
			return nil, errors.New("AS unavailable")
		}
		var x map[string]any
		if err = json.NewDecoder(res.Body).Decode(&x); err != nil {
			return nil, auth.ErrInvalidToken
		}
		active, _ := x["active"].(bool)
		if !active || x["iss"] != a.server.URL || x["aud"] != h.resource() {
			return nil, auth.ErrInvalidToken
		}
		subject, _ := x["sub"].(string)
		if !syntheticSubject(subject) {
			return nil, auth.ErrInvalidToken
		}
		client, _ := x["client_id"].(string)
		if client != "client-a" && client != "client-b" {
			return nil, auth.ErrInvalidToken
		}
		exp, _ := x["exp"].(float64)
		scopes, _ := x["scope"].(string)
		return &auth.TokenInfo{UserID: subject, Scopes: strings.Fields(scopes), Expiration: time.Unix(int64(exp), 0)}, nil
	}
	bearerOptions := &auth.RequireBearerTokenOptions{Scopes: []string{scope}}
	protect := auth.RequireBearerToken(verify, bearerOptions)
	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/oauth-protected-resource", func(w http.ResponseWriter, r *http.Request) {
		auth.ProtectedResourceMetadataHandler(&oauthex.ProtectedResourceMetadata{Resource: h.resource(), AuthorizationServers: []string{a.server.URL}, ScopesSupported: []string{scope}}).ServeHTTP(w, r)
	})
	path := "/mcp"
	if external {
		path = "/external-mcp"
	}
	limited := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		subject := auth.TokenInfoFromContext(r.Context()).UserID
		h.mu.Lock()
		if h.inflight[subject] >= 2 || h.totalInflight >= 8 {
			h.mu.Unlock()
			w.Header().Set("Retry-After", "1")
			http.Error(w, "synthetic inflight budget exceeded", 429)
			return
		}
		h.inflight[subject]++
		h.totalInflight++
		h.mu.Unlock()
		defer func() { h.mu.Lock(); h.inflight[subject]--; h.totalInflight--; h.mu.Unlock() }()
		stream.ServeHTTP(w, r)
	})
	protected := protect(limited)
	mux.Handle(path, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected, _ := url.Parse(h.server.URL)
		if r.Host != expected.Host || (r.Header.Get("Origin") != "" && r.Header.Get("Origin") != h.server.URL) {
			http.Error(w, "forbidden origin/host", 403)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 65537))
		r.Body = io.NopCloser(bytes.NewReader(body))
		var packet map[string]any
		if json.Unmarshal(body, &packet) == nil {
			params, _ := packet["params"].(map[string]any)
			meta, _ := params["_meta"].(map[string]any)
			caps, _ := meta["io.modelcontextprotocol/clientCapabilities"].(map[string]any)
			h.mu.Lock()
			h.observedCaps = append(h.observedCaps, caps)
			h.mu.Unlock()
		}
		protected.ServeHTTP(w, r)
	}))
	h.server = httptest.NewServer(mux)
	bearerOptions.ResourceMetadataURL = h.server.URL + "/.well-known/oauth-protected-resource"
	t.Cleanup(h.server.Close)
	return h
}
func (h *system) resource() string {
	if h.external {
		return h.server.URL + "/external-mcp"
	}
	return h.server.URL + "/mcp"
}
func envelope(data any) map[string]any {
	return map[string]any{"contract_version": "1.0.0", "request_id": "synthetic-request", "source_ref": "mock-reference", "observed_at": "2026-11-13T00:00:00Z", "warnings": []string{}, "status": "ok", "data": data, "error": nil}
}
func toolError(code int, message string) *mcp.CallToolResult {
	v := envelope(nil)
	v["status"] = "error"
	v["error"] = map[string]any{"code": code, "message": message, "retryable": false}
	b, _ := json.Marshal(v)
	return &mcp.CallToolResult{IsError: true, StructuredContent: v, Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}
}
func (h *system) handle(ctx context.Context, req *mcp.CallToolRequest, name string) (*mcp.CallToolResult, error) {
	h.calls.Add(1)
	var args map[string]any
	if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
		return toolError(10001, "invalid input"), nil
	}
	if err := h.schemas[name+"/inputSchema"].Validate(args); err != nil {
		return toolError(10001, "invalid input"), nil
	}
	if name == "zhuzhao_ticket_get_v1" {
		if _, err := strconv.ParseInt(args["ticket_id"].(string), 10, 64); err != nil {
			return toolError(10001, "invalid ticket id"), nil
		}
	}
	h.mu.Lock()
	overflow, oversized, denied, malicious, block := h.totalOverflow, h.oversized, h.remoteDenied, h.malicious, h.block
	h.mu.Unlock()
	if block != nil {
		h.entered <- struct{}{}
		select {
		case <-block:
		case <-ctx.Done():
			select {
			case h.cancelled <- struct{}{}:
			default:
			}
			return nil, ctx.Err()
		}
	}
	if malicious == "elicitation" {
		return &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{"unexpected": &mcp.ElicitParams{Message: "Synthetic request for unwanted interaction"}}, RequestState: "synthetic-only"}, nil
	}
	if malicious == "sampling" {
		return &mcp.CallToolResult{InputRequests: mcp.InputRequestMap{"unexpected": &mcp.CreateMessageParams{MaxTokens: 1}}, RequestState: "synthetic-only"}, nil
	}
	if denied {
		return toolError(70001, "remote access denied"), nil
	}
	subject := req.Extra.TokenInfo.UserID
	id := "1"
	if subject == "demo-b" {
		id = "2"
	}
	h.reads.Add(1)
	ticket := map[string]any{"id": id, "human_key": "DEMO-" + id, "title": "synthetic authorized ticket", "status": "open", "priority": 1, "updated_at": "2026-11-13T00:00:00Z", "link": "/tickets/" + id}
	var data any
	switch name {
	case "zhuzhao_ticket_list_v1":
		total := "1"
		if overflow {
			total = "9223372036854775808"
		}
		if _, err := strconv.ParseInt(total, 10, 64); err != nil {
			return toolError(10001, "output total out of contract range"), nil
		}
		data = map[string]any{"items": []any{ticket}, "total": total, "page": 1, "page_size": 20, "order": "id_desc", "consistency": "live_non_snapshot", "complete": true}
	case "zhuzhao_ticket_get_v1":
		if args["ticket_id"] != id {
			return toolError(10004, "not found"), nil
		}
		data = map[string]any{"ticket": ticket, "description_text": "synthetic public text", "description_truncated": false}
	case "mock_intel_lookup_v1":
		data = map[string]any{"indicator_type": args["indicator_type"], "normalized_value": args["value"], "verdict": "suspicious", "source_record_ref": "mock-record-1", "source_revision": "7", "fetched_at": "2026-11-13T00:00:00Z", "stale": false}
	}
	// Recheck the current local authority immediately before serializing any result.
	h.mu.Lock()
	stillAuthorized := h.local[subject] && !h.coreDown
	h.mu.Unlock()
	if !stillAuthorized {
		return toolError(70001, "permission withdrawn before response"), nil
	}
	output := envelope(data)
	if err := h.schemas[name+"/outputSchema"].Validate(output); err != nil {
		return toolError(10001, "output contract error"), nil
	}
	text, _ := json.Marshal(output)
	if oversized {
		text = bytes.Repeat(text, 1000)
	}
	result := &mcp.CallToolResult{StructuredContent: output, Content: []mcp.Content{&mcp.TextContent{Text: string(text)}}}
	encoded, _ := json.Marshal(result)
	if len(encoded) > 256<<10 {
		return toolError(10001, "result exceeds 256 KiB"), nil
	}
	return result, nil
}

type bearerTransport struct {
	token string
	owner *system
}

func (b bearerTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	clone := r.Clone(r.Context())
	clone.Header = r.Header.Clone()
	clone.Header.Set("Authorization", "Bearer "+b.token)
	response, err := http.DefaultTransport.RoundTrip(clone)
	if err == nil {
		b.owner.mu.Lock()
		b.owner.contentTypes = append(b.owner.contentTypes, response.Header.Get("Content-Type"))
		b.owner.mu.Unlock()
	}
	return response, err
}
func connect(t *testing.T, h *system, token *oauth2.Token) *mcp.ClientSession {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	client := mcp.NewClient(&mcp.Implementation{Name: "synthetic-client", Version: "1"}, &mcp.ClientOptions{Capabilities: &mcp.ClientCapabilities{}, MultiRoundTrip: &mcp.MultiRoundTripOptions{Disabled: true}})
	session, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: h.resource(), HTTPClient: &http.Client{Transport: bearerTransport{token.AccessToken, h}}}, &mcp.ClientSessionOptions{ProtocolVersion: revision})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = session.Close() })
	return session
}
func structured(t *testing.T, result *mcp.CallToolResult) map[string]any {
	t.Helper()
	b, _ := json.Marshal(result.StructuredContent)
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	return out
}
func errorCode(t *testing.T, result *mcp.CallToolResult) int {
	t.Helper()
	out := structured(t, result)
	e, ok := out["error"].(map[string]any)
	if !ok {
		t.Fatalf("no business error, result=%v", out)
	}
	n, _ := e["code"].(float64)
	return int(n)
}
func call(t *testing.T, s *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	result, err := s.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
func rawRPC(t *testing.T, h *system, token, method, name string, args any, mutate func(*http.Request, map[string]any)) (int, map[string]any, http.Header) {
	t.Helper()
	meta := map[string]any{"io.modelcontextprotocol/protocolVersion": revision, "io.modelcontextprotocol/clientCapabilities": map[string]any{}}
	params := map[string]any{"_meta": meta}
	if name != "" {
		params["name"] = name
		params["arguments"] = args
	}
	packet := map[string]any{"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
	req, _ := http.NewRequest("POST", h.resource(), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", revision)
	req.Header.Set("Mcp-Method", method)
	if name != "" {
		req.Header.Set("Mcp-Name", name)
	}
	if mutate != nil {
		mutate(req, packet)
	}
	body, _ := json.Marshal(packet)
	req.Body = io.NopCloser(bytes.NewReader(body))
	req.ContentLength = int64(len(body))
	response, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	b, _ := io.ReadAll(response.Body)
	var out map[string]any
	_ = json.Unmarshal(b, &out)
	return response.StatusCode, out, response.Header
}
func canonicalHash(v any) string {
	b, _ := json.Marshal(v)
	var value any
	_ = json.Unmarshal(b, &value)
	b, _ = json.Marshal(value)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

type consumeConfig struct {
	GlobalCache               bool
	Dataset                   string
	InputDigest, OutputDigest string
}

func consume(t *testing.T, h *system, token *oauth2.Token, cfg consumeConfig) (*mcp.CallToolResult, error) {
	t.Helper()
	if cfg.GlobalCache || cfg.Dataset != "approved_shared" {
		return nil, errors.New("external global cache disabled; shared dataset contract required")
	}
	session := connect(t, h, token)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	list, err := session.ListTools(ctx, nil)
	if err != nil {
		return nil, err
	}
	var tool *mcp.Tool
	for _, x := range list.Tools {
		if x.Name == "mock_intel_lookup_v1" {
			tool = x
		}
	}
	if tool == nil || canonicalHash(tool.InputSchema) != cfg.InputDigest || canonicalHash(tool.OutputSchema) != cfg.OutputDigest {
		return nil, errors.New("schema digest drift; mapping suspended")
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool.Name, Arguments: map[string]any{"indicator_type": "domain", "value": "indicator.example"}})
	if err != nil {
		return nil, err
	}
	if result.NeedsInput() {
		return nil, errors.New("server-initiated sampling/elicitation disabled")
	}
	if result.IsError {
		return nil, errors.New("external access or provider failure; not a not_found result")
	}
	return result, nil
}
func config(h *system) consumeConfig {
	x := h.bundle["mock_intel_lookup_v1"]
	return consumeConfig{Dataset: "approved_shared", InputDigest: canonicalHash(x["inputSchema"]), OutputDigest: canonicalHash(x["outputSchema"])}
}

func TestPublishJSONAndSSE(t *testing.T) {
	for _, jsonResponse := range []bool{true, false} {
		t.Run(fmt.Sprint(jsonResponse), func(t *testing.T) {
			a := newAS(t)
			h := newSystem(t, a, false, jsonResponse)
			for _, x := range []struct{ subject, client, id string }{{"demo-a", "client-a", "1"}, {"demo-b", "client-b", "2"}} {
				token := a.issue(t, x.subject, x.client, h.resource())
				session := connect(t, h, token)
				list, err := session.ListTools(context.Background(), nil)
				if err != nil || len(list.Tools) != 2 {
					t.Fatalf("tools=%v err=%v", list, err)
				}
				names := []string{list.Tools[0].Name, list.Tools[1].Name}
				sort.Strings(names)
				if strings.Join(names, ",") != "zhuzhao_ticket_get_v1,zhuzhao_ticket_list_v1" {
					t.Fatal(names)
				}
				result := call(t, session, "zhuzhao_ticket_list_v1", map[string]any{})
				if result.IsError {
					t.Fatal("list failed")
				}
				rows := structured(t, result)["data"].(map[string]any)["items"].([]any)
				if rows[0].(map[string]any)["id"] != x.id {
					t.Fatal("subject data mixed")
				}
				result = call(t, session, "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": x.id})
				if result.IsError {
					t.Fatal("get failed")
				}
			}
			h.mu.Lock()
			defer h.mu.Unlock()
			if h.methods["initialize"] != 0 {
				t.Fatal("unexpected initialize")
			}
			for _, caps := range h.observedCaps {
				if caps == nil || len(caps) != 0 {
					t.Fatalf("unexpected client capabilities %v", caps)
				}
			}
			want := "application/json"
			if !jsonResponse {
				want = "text/event-stream"
			}
			found := false
			for _, ct := range h.contentTypes {
				if strings.Contains(ct, want) {
					found = true
				}
			}
			if !found {
				t.Fatalf("missing transport %s", want)
			}
		})
	}
}
func TestSemanticCasesAll14(t *testing.T) {
	names := []string{"WRONG_AUDIENCE", "LOCAL_AT_OR_PAT", "SCOPE_WITHOUT_L1", "CROSS_ORG_GET", "AS_OR_CORE_AUTH_DOWN", "INPUT_ID_INT64_RANGE", "TOTAL_INT64_RANGE", "SCHEMA_DIGEST_DRIFT", "PRIVATE_DATA_WITH_SHARED_CACHE", "OVERSIZE_RESULT", "CONFIRM_WITHOUT_APPROVAL", "OAUTH_STATE_REPLAY", "ORIGIN_REBIND", "REMOTE_UNAUTHORIZED"}
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			a := newAS(t)
			external := name == "SCHEMA_DIGEST_DRIFT" || name == "PRIVATE_DATA_WITH_SHARED_CACHE" || name == "REMOTE_UNAUTHORIZED"
			h := newSystem(t, a, external, true)
			token := a.issue(t, "demo-a", "client-a", h.resource())
			switch name {
			case "WRONG_AUDIENCE":
				token = a.issue(t, "demo-a", "client-a", "http://127.0.0.1/other-resource")
				status, _, _ := rawRPC(t, h, token.AccessToken, "tools/list", "", nil, nil)
				if status != 401 || h.calls.Load() != 0 {
					t.Fatal(status)
				}
			case "LOCAL_AT_OR_PAT":
				for _, bad := range []string{"core-at-demo", "zpat_demo", ""} {
					status, _, _ := rawRPC(t, h, bad, "tools/list", "", nil, nil)
					if status != 401 {
						t.Fatal(status)
					}
				}
				if h.calls.Load() != 0 {
					t.Fatal("tool executed")
				}
			case "SCOPE_WITHOUT_L1":
				s := connect(t, h, token)
				h.mu.Lock()
				h.local["demo-a"] = false
				h.mu.Unlock()
				list, err := s.ListTools(context.Background(), nil)
				if err != nil || len(list.Tools) != 0 {
					t.Fatalf("%v %v", list, err)
				}
				if errorCode(t, call(t, s, "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"})) != 70001 || h.reads.Load() != 0 {
					t.Fatal("revoked L1 allowed")
				}
			case "CROSS_ORG_GET":
				s := connect(t, h, token)
				hidden := call(t, s, "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "2"})
				missing := call(t, s, "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "999"})
				if errorCode(t, hidden) != 10004 || errorCode(t, missing) != 10004 || canonicalHash(hidden.StructuredContent) != canonicalHash(missing.StructuredContent) {
					t.Fatal("existence leaked")
				}
			case "AS_OR_CORE_AUTH_DOWN":
				a.mu.Lock()
				a.down = true
				a.mu.Unlock()
				status, _, _ := rawRPC(t, h, token.AccessToken, "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"}, nil)
				if status < 400 || h.reads.Load() != 0 {
					t.Fatal("AS failed open")
				}
				a.mu.Lock()
				a.down = false
				a.mu.Unlock()
				s := connect(t, h, token)
				h.mu.Lock()
				h.coreDown = true
				h.mu.Unlock()
				if errorCode(t, call(t, s, "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"})) != 92601 || h.reads.Load() != 0 {
					t.Fatal("core auth failed open")
				}
			case "INPUT_ID_INT64_RANGE":
				s := connect(t, h, token)
				if errorCode(t, call(t, s, "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "9223372036854775808"})) != 10001 || h.reads.Load() != 0 {
					t.Fatal("overflow reached data")
				}
			case "TOTAL_INT64_RANGE":
				h.totalOverflow = true
				s := connect(t, h, token)
				r := call(t, s, "zhuzhao_ticket_list_v1", map[string]any{})
				if errorCode(t, r) != 10001 {
					t.Fatal("overflow output accepted")
				}
				b, _ := json.Marshal(r)
				if bytes.Contains(b, []byte("9223372036854775808")) {
					t.Fatal("overflow forwarded")
				}
			case "SCHEMA_DIGEST_DRIFT":
				cfg := config(h)
				h.drift = true
				if _, err := consume(t, h, token, cfg); err == nil || h.calls.Load() != 0 {
					t.Fatal("schema drift called tool")
				}
			case "PRIVATE_DATA_WITH_SHARED_CACHE":
				cfg := config(h)
				cfg.Dataset = "per_subject"
				cfg.GlobalCache = true
				if _, err := consume(t, h, token, cfg); err == nil || h.calls.Load() != 0 {
					t.Fatal("private global cache allowed")
				}
			case "OVERSIZE_RESULT":
				h.oversized = true
				s := connect(t, h, token)
				r := call(t, s, "zhuzhao_ticket_list_v1", map[string]any{})
				b, _ := json.Marshal(r)
				if errorCode(t, r) != 10001 || len(b) > 256<<10 {
					t.Fatal("oversize forwarded")
				}
			case "CONFIRM_WITHOUT_APPROVAL":
				s := connect(t, h, token)
				_, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "ticket_create_confirm", Arguments: map[string]any{"approved": true}})
				if err == nil || h.calls.Load() != 0 {
					t.Fatal("unregistered write tool allowed")
				}
			case "OAUTH_STATE_REPLAY":
				f, callback := a.begin(t, "demo-a", "client-b", h.resource())
				wrong := *callback
				q := wrong.Query()
				q.Set("state", "forged")
				wrong.RawQuery = q.Encode()
				before := a.exchanges
				if _, err := f.finish(&wrong); err == nil || a.exchanges != before {
					t.Fatal("wrong state exchanged")
				}
				good, err := f.finish(callback)
				if err != nil || good == nil {
					t.Fatal(err)
				}
				if _, err = f.finish(callback); err == nil {
					t.Fatal("state replay exchanged")
				}
				g, cb := a.begin(t, "demo-a", "client-a", h.resource())
				g.verifier = "wrong-pkce-verifier"
				if _, err = g.finish(cb); err == nil {
					t.Fatal("wrong PKCE exchanged")
				}
			case "ORIGIN_REBIND":
				status, _, _ := rawRPC(t, h, token.AccessToken, "tools/list", "", nil, func(r *http.Request, _ map[string]any) { r.Header.Set("Origin", "https://untrusted.example") })
				if status != 403 {
					t.Fatal(status)
				}
				status, _, _ = rawRPC(t, h, token.AccessToken, "tools/list", "", nil, func(r *http.Request, _ map[string]any) { r.Host = "untrusted.example" })
				if status != 403 || h.calls.Load() != 0 {
					t.Fatal(status)
				}
			case "REMOTE_UNAUTHORIZED":
				h.remoteDenied = true
				if _, err := consume(t, h, token, config(h)); err == nil || h.calls.Load() != 1 {
					t.Fatal("upstream denial became empty success")
				}
			}
		})
	}
}
func TestExternalReadSuccess(t *testing.T) {
	a := newAS(t)
	h := newSystem(t, a, true, true)
	token := a.issue(t, "demo-a", "client-a", h.resource())
	r, err := consume(t, h, token, config(h))
	if err != nil {
		t.Fatal(err)
	}
	if structured(t, r)["data"].(map[string]any)["verdict"] != "suspicious" {
		t.Fatal("wrong external result")
	}
}
func TestProtocolGuards(t *testing.T) {
	a := newAS(t)
	h := newSystem(t, a, false, true)
	token := a.issue(t, "demo-a", "client-a", h.resource())
	tests := []struct {
		name string
		mut  func(*http.Request, map[string]any)
		want int
	}{
		{"unsupported_version", func(r *http.Request, p map[string]any) {
			r.Header.Set("MCP-Protocol-Version", "2099-01-01")
			p["params"].(map[string]any)["_meta"].(map[string]any)["io.modelcontextprotocol/protocolVersion"] = "2099-01-01"
		}, -32022},
		{"missing_capabilities", func(_ *http.Request, p map[string]any) {
			delete(p["params"].(map[string]any)["_meta"].(map[string]any), "io.modelcontextprotocol/clientCapabilities")
		}, -32602},
		{"header_body_mismatch", func(r *http.Request, _ map[string]any) { r.Header.Set("Mcp-Name", "different-tool") }, -32020},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, result, _ := rawRPC(t, h, token.AccessToken, "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"}, test.mut)
			e, ok := result["error"].(map[string]any)
			if status != 400 || !ok || int(e["code"].(float64)) != test.want {
				t.Fatalf("status=%d result=%v", status, result)
			}
			if test.name == "unsupported_version" {
				data, ok := e["data"].(map[string]any)
				if !ok || data["requested"] != "2099-01-01" || canonicalHash(data["supported"]) != canonicalHash([]string{revision}) {
					t.Fatalf("supported versions absent or incorrect: %v", e)
				}
			}
		})
	}
	status, result, headers := rawRPC(t, h, token.AccessToken, "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"}, nil)
	if headers.Get("Mcp-Session-Id") != "" {
		t.Fatal("stateless response created a session")
	}
	if status != 200 || result["result"].(map[string]any)["resultType"] != "complete" {
		t.Fatal("modern resultType absent")
	}
	before := h.calls.Load()
	status, _, _ = rawRPC(t, h, token.AccessToken, "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1", "padding": strings.Repeat("x", 70<<10)}, nil)
	if status != 413 || h.calls.Load() != before {
		t.Fatal("body budget missing", status)
	}
	for _, method := range []string{"GET", "DELETE"} {
		req, _ := http.NewRequest(method, h.resource(), nil)
		req.Header.Set("Authorization", "Bearer "+token.AccessToken)
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		if res.StatusCode != 405 {
			t.Fatalf("%s=%d", method, res.StatusCode)
		}
	}
}
func TestNoAutomaticSamplingOrElicitation(t *testing.T) {
	for _, kind := range []string{"sampling", "elicitation"} {
		t.Run(kind, func(t *testing.T) {
			a := newAS(t)
			h := newSystem(t, a, true, true)
			h.malicious = kind
			token := a.issue(t, "demo-a", "client-a", h.resource())
			_, err := consume(t, h, token, config(h))
			if err == nil {
				t.Fatal("unwanted interaction not rejected")
			}
			if h.calls.Load() != 1 {
				t.Fatalf("automatic MRTR retry occurred: %d", h.calls.Load())
			}
		})
	}
}
func TestCancellationReachesHandler(t *testing.T) {
	a := newAS(t)
	h := newSystem(t, a, true, false)
	token := a.issue(t, "demo-a", "client-a", h.resource())
	s := connect(t, h, token)
	h.block = make(chan struct{})
	ctx, cancel := context.WithTimeout(context.Background(), 80*time.Millisecond)
	defer cancel()
	_, err := s.CallTool(ctx, &mcp.CallToolParams{Name: "mock_intel_lookup_v1", Arguments: map[string]any{"indicator_type": "domain", "value": "indicator.example"}})
	if err == nil {
		t.Fatal("deadline ignored")
	}
	select {
	case <-h.cancelled:
	case <-time.After(2 * time.Second):
		t.Fatal("HTTP cancellation did not reach handler")
	}
}

func TestBearerIdentityFailures(t *testing.T) {
	for _, name := range []string{"revoked", "expired", "wrong_issuer", "unknown_subject", "unknown_client", "insufficient_scope"} {
		t.Run(name, func(t *testing.T) {
			a := newAS(t)
			h := newSystem(t, a, false, true)
			token := a.issue(t, "demo-a", "client-a", h.resource())
			status, _, _ := rawRPC(t, h, token.AccessToken, "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"}, nil)
			if status != 200 || h.reads.Load() != 1 {
				t.Fatal("baseline token failed")
			}
			a.mu.Lock()
			record := a.tokens[token.AccessToken]
			switch name {
			case "revoked":
				record.Active = false
			case "expired":
				record.Expiry = time.Now().Add(-time.Minute).Unix()
			case "wrong_issuer":
				record.Issuer = "https://untrusted.example"
			case "unknown_subject":
				record.Subject = "demo-outside"
			case "unknown_client":
				record.Client = "foreign-client"
			case "insufficient_scope":
				record.Scope = "openid"
			}
			a.mu.Unlock()
			status, _, header := rawRPC(t, h, token.AccessToken, "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"}, nil)
			want := 401
			if name == "insufficient_scope" {
				want = 403
			}
			if status != want || h.reads.Load() != 1 || h.calls.Load() != 1 {
				t.Fatalf("auth failed open: status=%d calls=%d reads=%d", status, h.calls.Load(), h.reads.Load())
			}
			if !strings.Contains(header.Get("WWW-Authenticate"), h.server.URL+"/.well-known/oauth-protected-resource") {
				t.Fatal("metadata challenge absent")
			}
		})
	}
}
func TestAuthorizationWithdrawnBeforeResponse(t *testing.T) {
	a := newAS(t)
	h := newSystem(t, a, false, true)
	token := a.issue(t, "demo-a", "client-a", h.resource())
	s := connect(t, h, token)
	block := make(chan struct{})
	h.mu.Lock()
	h.block = block
	h.mu.Unlock()
	released := false
	defer func() {
		if !released {
			close(block)
		}
	}()
	type outcome struct {
		r   *mcp.CallToolResult
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		r, err := s.CallTool(context.Background(), &mcp.CallToolParams{Name: "zhuzhao_ticket_get_v1", Arguments: map[string]any{"ticket_id": "1"}})
		done <- outcome{r, err}
	}()
	select {
	case <-h.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not enter")
	}
	h.mu.Lock()
	h.local["demo-a"] = false
	h.mu.Unlock()
	close(block)
	released = true
	select {
	case result := <-done:
		if result.err != nil {
			t.Fatal(result.err)
		}
		if errorCode(t, result.r) != 70001 {
			t.Fatal("withdrawal not observed")
		}
		b, _ := json.Marshal(result.r)
		if bytes.Contains(b, []byte("synthetic authorized ticket")) {
			t.Fatal("revoked response exposed ticket")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("request did not finish")
	}
}
func TestInflightBudgets(t *testing.T) {
	for _, connection := range []bool{false, true} {
		t.Run(fmt.Sprint(connection), func(t *testing.T) {
			a := newAS(t)
			h := newSystem(t, a, false, true)
			tokens := map[string]string{}
			for _, subject := range []string{"demo-a", "demo-b", "demo-c", "demo-d", "demo-e"} {
				tokens[subject] = a.issue(t, subject, "client-a", h.resource()).AccessToken
			}
			block := make(chan struct{})
			h.mu.Lock()
			h.block = block
			h.mu.Unlock()
			released := false
			defer func() {
				if !released {
					close(block)
				}
			}()
			subjects := []string{"demo-a", "demo-a"}
			deniedSubject := "demo-a"
			if connection {
				subjects = []string{"demo-a", "demo-a", "demo-b", "demo-b", "demo-c", "demo-c", "demo-d", "demo-d"}
				deniedSubject = "demo-e"
			}
			// All requests use the real HTTP/SDK server. The semaphore is a local reference adapter.
			done := make(chan int, len(subjects))
			for _, subject := range subjects {
				subject := subject
				go func() {
					status, _, _ := rawRPC(t, h, tokens[subject], "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"}, nil)
					done <- status
				}()
			}
			for range subjects {
				select {
				case <-h.entered:
				case <-time.After(3 * time.Second):
					t.Fatal("inflight handlers did not enter")
				}
			}
			before := h.calls.Load()
			status, _, headers := rawRPC(t, h, tokens[deniedSubject], "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"}, nil)
			if status != 429 || headers.Get("Retry-After") != "1" || h.calls.Load() != before {
				t.Fatalf("inflight budget failed: %d calls=%d", status, h.calls.Load())
			}
			close(block)
			released = true
			for range subjects {
				select {
				case status := <-done:
					if status != 200 {
						t.Fatal(status)
					}
				case <-time.After(3 * time.Second):
					t.Fatal("budget not released")
				}
			}
			h.mu.Lock()
			h.block = nil
			left := h.totalInflight
			h.mu.Unlock()
			if left != 0 {
				t.Fatal("permit leaked", left)
			}
			status, _, _ = rawRPC(t, h, tokens[deniedSubject], "tools/call", "zhuzhao_ticket_get_v1", map[string]any{"ticket_id": "1"}, nil)
			if status != 200 {
				t.Fatal("permit did not recover")
			}
		})
	}
}
func TestProtectedResourceMetadataAndLegacyBoundary(t *testing.T) {
	a := newAS(t)
	h := newSystem(t, a, false, true)
	token := a.issue(t, "demo-a", "client-a", h.resource())
	res, err := http.Get(h.server.URL + "/.well-known/oauth-protected-resource")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var metadata map[string]any
	if err = json.NewDecoder(res.Body).Decode(&metadata); err != nil {
		t.Fatal(err)
	}
	if metadata["resource"] != h.resource() || metadata["authorization_servers"].([]any)[0] != a.server.URL {
		t.Fatal("resource metadata identity mismatch")
	}
	status, body, _ := rawRPC(t, h, token.AccessToken, "tools/list", "", nil, func(r *http.Request, p map[string]any) {
		r.Header.Set("MCP-Protocol-Version", "1900-01-01")
		p["params"].(map[string]any)["_meta"].(map[string]any)["io.modelcontextprotocol/protocolVersion"] = "1900-01-01"
	})
	if status != 400 || len(body) != 0 {
		t.Fatalf("fixed SDK legacy boundary changed: %d %v", status, body)
	}
	if h.calls.Load() != 0 {
		t.Fatal("unsupported legacy request executed")
	}
}

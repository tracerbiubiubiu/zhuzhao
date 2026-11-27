package referencequery

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

const maxWireBytes = 1 << 20

type Peer struct {
	origin string
	ak     string
	sk     []byte
	http   *http.Client
}

func NewPeer(origin, ak, sk string, timeout time.Duration) (*Peer, error) {
	u, err := url.Parse(origin)
	if err != nil || u.Hostname() == "" || u.User != nil || u.Opaque != "" || u.EscapedPath() != "" && u.EscapedPath() != "/" ||
		u.RawQuery != "" || u.ForceQuery || strings.Contains(origin, "#") || timeout <= 0 || !validText(ak, 128) || strings.ContainsAny(ak, ",= ") || sk == "" {
		return nil, &Failure{Status: 400, Code: "invalid_input"}
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (net.ParseIP(u.Hostname()).IsLoopback() || u.Hostname() == "localhost")) {
		return nil, &Failure{Status: 400, Code: "invalid_input"}
	}
	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return nil, &Failure{Status: 503, Code: "source_unavailable"}
	}
	transport := base.Clone()
	transport.Proxy = nil
	transport.MaxConnsPerHost = 16
	client := &http.Client{
		Timeout:       timeout,
		Transport:     transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &Peer{origin: u.Scheme + "://" + u.Host, ak: ak, sk: []byte(sk), http: client}, nil
}
func validRoute(route string) bool {
	return route == AppPath || route == AdmissionPath || route == ConnectorPath
}
func statusFailure(status int) *Failure {
	switch status {
	case 400:
		return &Failure{Status: 400, Code: "invalid_input"}
	case 401, 403:
		return &Failure{Status: status, Code: "not_authorized"}
	case 409:
		return &Failure{Status: 409, Code: "contract_mismatch"}
	case 429:
		return &Failure{Status: 429, Code: "quota"}
	default:
		return &Failure{Status: 503, Code: "source_unavailable"}
	}
}
func requestID(request any) string {
	switch v := request.(type) {
	case Invocation:
		return v.RequestID
	case *Invocation:
		if v != nil {
			return v.RequestID
		}
	}
	return ""
}
func (p *Peer) Post(ctx context.Context, route string, request, response any) error {
	if !validRoute(route) {
		return statusFailure(400)
	}
	payload, err := json.Marshal(request)
	if err != nil || len(payload) > maxWireBytes {
		return statusFailure(400)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, p.origin+route, bytes.NewReader(payload))
	if err != nil {
		return statusFailure(400)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	aksk.Sign(req, payload, aksk.SignOptions{AK: p.ak, SK: p.sk, RequestID: requestID(request)})
	res, err := p.http.Do(req)
	if err != nil {
		return statusFailure(503)
	}
	defer res.Body.Close()
	b, err := io.ReadAll(io.LimitReader(res.Body, maxWireBytes+1))
	if err != nil || len(b) > maxWireBytes {
		return statusFailure(503)
	}
	if res.StatusCode != http.StatusOK {
		// This request authenticates a peer service. Its rejection describes
		// an unavailable dependency and must not become a user's session error.
		if res.StatusCode == http.StatusUnauthorized {
			return statusFailure(http.StatusServiceUnavailable)
		}
		f := statusFailure(res.StatusCode)
		// Only public standard classifications may cross the service boundary.
		var body Failure
		if json.Unmarshal(b, &body) == nil && (body.Code == f.Code || (res.StatusCode == 409 && body.Code == "unknown_enum") || (res.StatusCode == 503 && body.Code == "source_stale")) {
			f.Code = body.Code
			if body.RetryAfterMS != nil && *body.RetryAfterMS >= 0 {
				f.RetryAfterMS = body.RetryAfterMS
			}
		}
		return f
	}
	if !validObject(b) || json.Unmarshal(b, response) != nil {
		return statusFailure(409)
	}
	if page, ok := response.(*QueryPage); ok {
		var keys map[string]json.RawMessage
		if json.Unmarshal(b, &keys) != nil || string(keys["complete"]) != "true" && string(keys["complete"]) != "false" || page.Items == nil {
			return statusFailure(409)
		}
	}
	return nil
}

type Client struct{ peer *Peer }

func NewClient(origin, ak, sk string, timeout time.Duration) (*Client, error) {
	p, err := NewPeer(origin, ak, sk, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{peer: p}, nil
}
func (c *Client) Query(ctx context.Context, q QueryRequest) (QueryPage, error) {
	var p QueryPage
	if err := ValidateRequest(q); err != nil {
		return QueryPage{}, err
	}
	if err := c.peer.Post(ctx, ConnectorPath, q, &p); err != nil {
		return QueryPage{}, err
	}
	if err := ValidatePage(q, p); err != nil {
		return QueryPage{}, err
	}
	return p, nil
}

// ReadSigned accepts only a canonical JSON request for the configured service route.
func ReadSigned(r *http.Request, verifier *aksk.Verifier, route string, dst any) *Failure {
	if !validRoute(route) || r.Method != "POST" || r.URL.EscapedPath() != route || r.URL.RawQuery != "" || r.URL.ForceQuery {
		return statusFailure(400)
	}
	if len(r.Header.Values(aksk.HeaderXOperator)) != 0 || len(r.Header.Values(aksk.HeaderAuthorization)) != 1 || len(r.Header.Values(aksk.HeaderXRequestID)) > 1 {
		return statusFailure(401)
	}
	b, err := io.ReadAll(io.LimitReader(r.Body, maxWireBytes+1))
	if err != nil || len(b) > maxWireBytes {
		return statusFailure(400)
	}
	if verifier == nil || verifier.Verify(r, b) != nil {
		return statusFailure(401)
	}
	if !validObject(b) {
		return statusFailure(400)
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return statusFailure(400)
	}
	if v, ok := dst.(*Invocation); ok && (v.RequestID == "" || v.RequestID != r.Header.Get("X-Request-ID")) {
		return statusFailure(403)
	}
	return nil
}
func WriteFailure(w http.ResponseWriter, f *Failure) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(f.Status)
	_ = json.NewEncoder(w).Encode(f)
}

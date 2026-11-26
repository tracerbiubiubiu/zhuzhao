package riskasset

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
)

// Config uses explicit directory-only credentials; no environment or shared credential fallback.
type Config struct {
	BaseURL          string
	AK               string
	SK               []byte
	Timeout          time.Duration
	MaxAge           time.Duration
	MaxResponseBytes int64
	Now              func() time.Time
	Transport        http.RoundTripper
}

// Client implements Port over the isolated v1 fixture protocol.
type Client struct {
	endpoint         string
	ak               string
	sk               []byte
	timeout          time.Duration
	maxAge           time.Duration
	maxResponseBytes int64
	now              func() time.Time
	http             *http.Client
}

var _ Port = (*Client)(nil)

// NewClient constructs a directory client without performing I/O.
func NewClient(cfg Config) (*Client, error) {
	u, err := url.Parse(cfg.BaseURL)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") ||
		u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" ||
		!validKey(cfg.AK, 128) || strings.ContainsAny(cfg.AK, ",= ") || len(cfg.SK) == 0 ||
		cfg.Timeout <= 0 || cfg.MaxAge <= 0 || cfg.MaxResponseBytes < 0 || cfg.MaxResponseBytes > 1<<20 {
		return nil, ErrInvalidConfig
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.MaxResponseBytes == 0 {
		cfg.MaxResponseBytes = 64 << 10
	}
	if cfg.Transport == nil {
		base, ok := http.DefaultTransport.(*http.Transport)
		if !ok {
			return nil, ErrInvalidConfig
		}
		direct := base.Clone()
		direct.Proxy = nil
		cfg.Transport = direct
	}
	u.Path = "/v1/assets/lookup"
	u.RawPath = ""
	return &Client{
		endpoint: u.String(), ak: cfg.AK, sk: append([]byte(nil), cfg.SK...),
		timeout: cfg.Timeout, maxAge: cfg.MaxAge, maxResponseBytes: cfg.MaxResponseBytes, now: cfg.Now,
		http: &http.Client{Transport: cfg.Transport, CheckRedirect: func(*http.Request, []*http.Request) error {
			// Redirects can change the signed target or forward credentials. Never follow them.
			return http.ErrUseLastResponse
		}},
	}, nil
}

// Lookup fetches one directory record without mutating any local business state.
func (c *Client) Lookup(ctx context.Context, input LookupRequest) (LookupResult, error) {
	if err := ctx.Err(); err != nil {
		return failed(err)
	}
	if !validKey(input.AssetRef.Provider, 256) || !validKey(input.AssetRef.ExternalID, 256) || !validKey(input.RequestID, 128) {
		return LookupResult{Status: StatusUnavailable}, ErrInvalidRequest
	}
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	payload, err := json.Marshal(input)
	if err != nil {
		return failed(err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return failed(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	aksk.Sign(req, payload, aksk.SignOptions{AK: c.ak, SK: c.sk, RequestID: input.RequestID, Now: c.now})
	resp, err := c.http.Do(req)
	if err != nil {
		return failed(contextError(ctx, err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return failed(contextError(ctx, ErrUnavailable))
	}
	// Read one extra byte to reject oversized bodies instead of accepting a truncated prefix.
	raw, err := io.ReadAll(io.LimitReader(resp.Body, c.maxResponseBytes+1))
	if err != nil {
		return failed(contextError(ctx, err))
	}
	if err := ctx.Err(); err != nil {
		return failed(err)
	}
	if int64(len(raw)) > c.maxResponseBytes {
		return failed(ErrUnavailable)
	}
	var out struct {
		Status Status `json:"status"`
		Evidence
	}
	// Unmarshal consumes the entire document; arrays, malformed/trailing JSON are rejected.
	if !validResponseText(raw) || json.Unmarshal(raw, &out) != nil || out.AssetRef != input.AssetRef || out.ObservedAt.IsZero() {
		return failed(ErrUnavailable)
	}
	now := c.now()
	// Do not publish a late result after the caller has abandoned this lookup.
	if err := ctx.Err(); err != nil {
		return failed(err)
	}
	if out.ObservedAt.After(now) {
		return failed(ErrUnavailable)
	}
	age := now.Sub(out.ObservedAt)
	switch out.Status {
	case StatusNotFound:
		// An old absence is not proof that this identity currently does not exist.
		if age > c.maxAge {
			return failed(ErrUnavailable)
		}
		return LookupResult{Status: StatusNotFound}, nil
	case StatusOK, StatusStale:
		if age > c.maxAge {
			out.Status = StatusStale
		}
		if out.Snapshot.Components == nil {
			out.Snapshot.Components = []Component{}
		}
		return LookupResult{Status: out.Status, Evidence: &out.Evidence}, nil
	default:
		return failed(ErrUnavailable)
	}
}

func validKey(value string, limit int) bool {
	if value == "" || len(value) > limit || !utf8.ValidString(value) || strings.TrimSpace(value) != value {
		return false
	}
	return !strings.ContainsFunc(value, unicode.IsControl)
}

// contextError preserves cancellation even when a transport reports another error.
func contextError(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	return err
}

func failed(err error) (LookupResult, error) {
	if errors.Is(err, context.Canceled) {
		return LookupResult{Status: StatusCanceled}, context.Canceled
	}
	var timeout net.Error
	if errors.Is(err, context.DeadlineExceeded) || errors.As(err, &timeout) && timeout.Timeout() {
		return LookupResult{Status: StatusTimeout}, context.DeadlineExceeded
	}
	// No upstream body, URL, credentials or raw transport detail escapes this boundary.
	return LookupResult{Status: StatusUnavailable}, ErrUnavailable
}

package riskasset

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type countingBody struct {
	io.Reader
	read   int
	closed bool
}

func (b *countingBody) Read(p []byte) (int, error) {
	n, e := b.Reader.Read(p)
	b.read += n
	return n, e
}
func (b *countingBody) Close() error { b.closed = true; return nil }

func TestLookupBodyExactLimitAndBoundedRead(t *testing.T) {
	payload, _ := json.Marshal(struct {
		Status Status `json:"status"`
		Evidence
	}{StatusOK, fixtureEvidence("asset-A")})
	for _, extra := range []int{0, 1, 2048} {
		t.Run(strings.Repeat("x", min(extra, 2)), func(t *testing.T) {
			body := &countingBody{Reader: strings.NewReader(string(payload) + strings.Repeat(" ", extra))}
			c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("transport should be synthetic") }, func(cfg *Config) {
				cfg.MaxResponseBytes = int64(len(payload))
				cfg.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
					return &http.Response{StatusCode: 200, Body: body, Header: make(http.Header)}, nil
				})
			})
			got, err := c.Lookup(context.Background(), LookupRequest{fixtureEvidence("asset-A").AssetRef, "synthetic-limit"})
			if extra == 0 {
				if got.Status != StatusOK || err != nil {
					t.Fatalf("exact limit rejected: %+v, %v", got, err)
				}
			} else {
				if got.Status != StatusUnavailable || !errors.Is(err, ErrUnavailable) {
					t.Fatalf("oversize accepted: %+v, %v", got, err)
				}
			}
			if !body.closed || body.read > len(payload)+1 {
				t.Fatalf("body closed=%v, consumed=%d, limit=%d", body.closed, body.read, len(payload))
			}
		})
	}
}

func TestNewClientConfigValidation(t *testing.T) {
	base := Config{BaseURL: "http://127.0.0.1:1", AK: fixtureAK, SK: []byte(fixtureSK), Timeout: time.Second, MaxAge: time.Minute}
	for _, tc := range []struct {
		name   string
		change func(*Config)
	}{
		{"empty_endpoint", func(c *Config) { c.BaseURL = "" }}, {"relative", func(c *Config) { c.BaseURL = "/lookup" }},
		{"bad_scheme", func(c *Config) { c.BaseURL = "file:///synthetic" }}, {"embedded_auth", func(c *Config) { c.BaseURL = "http://synthetic:secret@127.0.0.1" }},
		{"path", func(c *Config) { c.BaseURL += "/path" }}, {"query", func(c *Config) { c.BaseURL += "?x=1" }}, {"fragment", func(c *Config) { c.BaseURL += "#x" }},
		{"no_ak", func(c *Config) { c.AK = "" }}, {"bad_ak", func(c *Config) { c.AK = "caller,Ts=x" }}, {"no_sk", func(c *Config) { c.SK = nil }},
		{"no_timeout", func(c *Config) { c.Timeout = 0 }}, {"no_age", func(c *Config) { c.MaxAge = 0 }},
		{"negative_bound", func(c *Config) { c.MaxResponseBytes = -1 }}, {"unbounded", func(c *Config) { c.MaxResponseBytes = 1<<20 + 1 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg := base
			tc.change(&cfg)
			c, err := NewClient(cfg)
			if c != nil || !errors.Is(err, ErrInvalidConfig) {
				t.Fatalf("got %v, %v", c, err)
			}
		})
	}
	for _, suffix := range []string{"", "/"} {
		cfg := base
		cfg.BaseURL += suffix
		if _, err := NewClient(cfg); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLookupInvalidUTF8NeverSends(t *testing.T) {
	c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { t.Error("invalid UTF-8 reached server") }, nil)
	got, err := c.Lookup(context.Background(), LookupRequest{Ref{"synthetic-cmdb", string([]byte{0xff})}, "synthetic-invalid"})
	if !errors.Is(err, ErrInvalidRequest) || got.Evidence != nil {
		t.Fatalf("got %+v, %v; want invalid request", got, err)
	}
}

func TestLookupInvalidResponseUnicodeCannotAliasIdentity(t *testing.T) {
	e := fixtureEvidence("\ufffd")
	payload, _ := json.Marshal(struct {
		Status Status `json:"status"`
		Evidence
	}{StatusOK, e})
	for _, encoding := range []string{string([]byte{0xff}), `\ud800`, `\udc00`} {
		malformed := strings.ReplaceAll(string(payload), "\ufffd", encoding)
		c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(malformed)) }, nil)
		got, err := c.Lookup(context.Background(), LookupRequest{e.AssetRef, "synthetic-response-encoding"})
		if got.Status != StatusUnavailable || got.Evidence != nil || !errors.Is(err, ErrUnavailable) {
			t.Errorf("invalid encoding aliased identity: %+v, %v", got, err)
		}
	}
}

func TestLookupValidResponseUnicodeControls(t *testing.T) {
	for _, id := range []string{"\ufffd", "\U0001f600", `asset-\ud800`} {
		e := fixtureEvidence(id)
		payload, _ := json.Marshal(struct {
			Status Status `json:"status"`
			Evidence
		}{StatusOK, e})
		encoded := strings.ReplaceAll(string(payload), "\U0001f600", `\ud83d\ude00`)
		c := fixtureClient(t, func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(encoded)) }, nil)
		got, err := c.Lookup(context.Background(), LookupRequest{e.AssetRef, "synthetic-valid-encoding"})
		if got.Status != StatusOK || err != nil || got.Evidence == nil || got.Evidence.AssetRef != e.AssetRef {
			t.Fatalf("valid encoding rejected: %+v, %v", got, err)
		}
	}
}

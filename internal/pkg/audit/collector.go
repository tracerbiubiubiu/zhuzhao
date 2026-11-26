package audit

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/http"
	"net/url"
	"time"
)

// CollectorKey binds pending records to the exact originally selected destination.
func CollectorKey(endpoint string) string {
	if endpoint == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(endpoint))
	return hex.EncodeToString(sum[:])
}

type HTTPCollector struct {
	endpoint, key string
	client        *http.Client
}

func NewHTTPCollector(endpoint string) (*HTTPCollector, error) {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || (u.Scheme != "https" && !(u.Scheme == "http" && net.ParseIP(u.Hostname()).IsLoopback())) {
		return nil, errors.New("invalid IAM audit collector configuration")
	}
	client := &http.Client{
		Timeout:       5 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	return &HTTPCollector{endpoint: endpoint, key: CollectorKey(endpoint), client: client}, nil
}

func (c *HTTPCollector) Key() string { return c.key }

func (c *HTTPCollector) Deliver(ctx context.Context, id string, payload []byte) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(payload))
	if err != nil {
		return errors.New("IAM audit request construction failed")
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", id)
	resp, err := c.client.Do(req)
	if err != nil {
		return errors.New("IAM audit delivery unavailable")
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errors.New("IAM audit delivery rejected")
	}
	return nil
}

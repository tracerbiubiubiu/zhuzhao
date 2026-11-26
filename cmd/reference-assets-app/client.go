package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Compatibility mode keeps its original public URL and response shape
// independent of the current referencequery Go types.
const frozenQueryPath = "/api/v1/extensions/reference-assets"

type frozenPage struct {
	Items          []map[string]json.RawMessage `json:"items"`
	NextCursor     string                       `json:"next_cursor"`
	Complete       *bool                        `json:"complete"`
	SourceID       string                       `json:"source_id"`
	ObservedAt     string                       `json:"observed_at"`
	FetchedAt      string                       `json:"fetched_at"`
	MappingVersion string                       `json:"mapping_version"`
	Warnings       []string                     `json:"warnings"`
}

func checkCore(origin, token, rawQuery string, timeout time.Duration, out io.Writer) error {
	target, err := url.Parse(origin)
	if err != nil || target.Hostname() == "" || target.User != nil || target.Opaque != "" ||
		(target.EscapedPath() != "" && target.EscapedPath() != "/") || target.RawQuery != "" ||
		target.ForceQuery || strings.Contains(origin, "#") {
		return errors.New("invalid Core origin")
	}
	if target.Scheme != "https" && (target.Scheme != "http" || !net.ParseIP(target.Hostname()).IsLoopback()) {
		return errors.New("Core origin must use HTTPS or loopback HTTP")
	}
	query, err := url.ParseQuery(rawQuery)
	if err != nil {
		return errors.New("invalid compatibility query")
	}
	target.Path = frozenQueryPath
	target.RawQuery = query.Encode()
	req, err := http.NewRequest(http.MethodGet, target.String(), nil)
	if err != nil {
		return errors.New("invalid compatibility request")
	}
	req.Header.Set("Accept", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	base, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		return errors.New("no HTTP transport available")
	}
	transport := base.Clone()
	transport.Proxy = nil
	defer transport.CloseIdleConnections()
	client := &http.Client{
		Timeout: timeout, Transport: transport,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	res, err := client.Do(req)
	if err != nil {
		return errors.New("Core HTTP request failed")
	}
	defer res.Body.Close()
	const responseLimit = 1 << 20
	raw, err := io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
	if err != nil || len(raw) > responseLimit {
		return errors.New("unable to read the bounded Core response")
	}

	if res.StatusCode != http.StatusOK {
		if json.Valid(raw) {
			_, _ = fmt.Fprintln(out, string(raw))
		}
		return fmt.Errorf("Core returned HTTP %d", res.StatusCode)
	}
	if !validFrozenPage(raw, query["requested_fields"]) {
		return errors.New("Core response is incompatible with the frozen query caller")
	}
	_, err = fmt.Fprintln(out, string(raw))
	return err
}

func validFrozenPage(raw []byte, requestedFields []string) bool {
	var page frozenPage
	if json.Unmarshal(raw, &page) != nil || page.Items == nil || page.Complete == nil ||
		page.SourceID == "" || page.MappingVersion == "" || page.Warnings == nil ||
		(*page.Complete && page.NextCursor != "") || (!*page.Complete && page.NextCursor == "" && len(page.Warnings) == 0) {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, page.ObservedAt); err != nil {
		return false
	}
	if _, err := time.Parse(time.RFC3339Nano, page.FetchedAt); err != nil {
		return false
	}
	for _, row := range page.Items {
		if row == nil {
			return false
		}
		for _, field := range requestedFields {
			value, ok := row[field]
			if !ok || !validFrozenValue(field, value) {
				return false
			}
		}
	}
	return true
}

func validFrozenValue(field string, value json.RawMessage) bool {
	switch field {
	case "stale":
		return string(value) == "true" || string(value) == "false"
	case "observed_at":
		if string(value) == "null" {
			return true
		}
		var stamp string
		if json.Unmarshal(value, &stamp) != nil {
			return false
		}
		_, err := time.Parse(time.RFC3339Nano, stamp)
		return err == nil
	case "device_id", "name", "owner_org_ref", "category", "status":
		var text string
		if json.Unmarshal(value, &text) != nil || text == "" {
			return false
		}
		return field != "status" || text == "healthy" || text == "degraded" || text == "down" || text == "unknown"
	default:
		return false
	}
}

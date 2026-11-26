package b3al

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao-utils/aksk"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/jobs"
)

const (
	updatePath    = "/internal/activelist/documents/update"
	receiptPath   = "/internal/activelist/documents/receipt"
	getPath       = "/internal/activelist/documents/get"
	responseLimit = 128 << 10
)

type Client struct {
	origin string
	http   *http.Client
}

func NewClient(baseURL, ak, sk string) (*Client, error) {
	u, err := url.Parse(baseURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || u.RawPath != "" || (u.Path != "" && u.Path != "/") || (u.Scheme != "https" && u.Scheme != "http") || strings.TrimSpace(ak) == "" || sk == "" {
		return nil, errors.New("invalid ActiveList service configuration")
	}
	u.Path = ""
	return &Client{origin: u.String(), http: &http.Client{Timeout: 10 * time.Second, Transport: &aksk.Transport{AK: ak, SK: []byte(sk)}, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}}, nil
}

// HTTPError retains the status without exposing ActiveList response content.
type HTTPError struct{ Status int }

func (e *HTTPError) Error() string { return fmt.Sprintf("ActiveList returned HTTP %d", e.Status) }

var errResponse = errors.New("invalid ActiveList response")

type ReadResult struct {
	ObservationOwner    string         `json:"observation_owner"`
	DataOwner           string         `json:"data_owner"`
	TypeName            string         `json:"type_name"`
	DocumentID          string         `json:"document_id"`
	Generation          string         `json:"generation"`
	DocumentRevision    string         `json:"document_revision"`
	QueriedAt           string         `json:"queried_at"`
	Values              map[string]any `json:"values"`
	SourceEventID       string         `json:"source_event_id,omitempty"`
	SourceEventRevision string         `json:"source_event_revision,omitempty"`
}

type UpdateResult struct {
	TypeName         string `json:"type_name"`
	DocumentID       string `json:"document_id"`
	Generation       string `json:"generation"`
	DocumentRevision string `json:"document_revision"`
	Noop             bool   `json:"noop"`
}

func exactNumber(s string) (any, bool) {
	if len(s) > 128 || !json.Valid([]byte(s)) {
		return nil, false
	}
	if i := strings.IndexAny(s, "eE"); i >= 0 {
		exponent, err := strconv.Atoi(s[i+1:])
		if err != nil || exponent < -256 || exponent > 256 {
			return nil, false
		}
	}
	v, ok := new(big.Rat).SetString(s)
	if !ok || !v.IsInt() || v.Num().BitLen() > 53 {
		return nil, false
	}
	return v.Num().Int64(), true
}

func (cl *Client) post(ctx context.Context, cmd Command, path string, output any) error {
	body, err := cmd.envelope(time.Now(), path)
	if err != nil {
		return err
	}
	raw, err := json.Marshal(body)
	if err != nil {
		return ErrInvalid
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cl.origin+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := cl.http.Do(req)
	if err != nil {
		return fmt.Errorf("ActiveList request failed: %w", err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return &HTTPError{Status: res.StatusCode}
	}
	raw, err = io.ReadAll(io.LimitReader(res.Body, responseLimit+1))
	if err != nil || len(raw) > responseLimit {
		return errResponse
	}
	if _, err = jobs.EqualParams(raw, raw); err != nil {
		return errResponse
	}
	var envelope struct {
		Code *int            `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil || envelope.Code == nil || *envelope.Code != 0 || len(envelope.Data) == 0 {
		return errResponse
	}
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.UseNumber()
	decoder.DisallowUnknownFields()
	if err = decoder.Decode(output); err != nil {
		return errResponse
	}
	return nil
}

func (cl *Client) Get(ctx context.Context, cmd Command) (ReadResult, error) {
	var out ReadResult
	if err := cl.post(ctx, cmd, getPath, &out); err != nil {
		return ReadResult{}, err
	}
	_, err := time.Parse(time.RFC3339Nano, out.QueriedAt)
	if err != nil || out.TypeName != cmd.TypeName || out.DocumentID != cmd.DocumentID || out.Generation != cmd.Generation || !b3protocol.PositiveNumber(out.DocumentRevision) || out.Values == nil {
		return ReadResult{}, errResponse
	}
	// Core owns observation and inbox metadata. The AL transport has no authority
	// to substitute another trigger revision or observation owner.
	if out.ObservationOwner != "" && out.ObservationOwner != "zhuzhao" || out.DataOwner != "" && out.DataOwner != "activelist" || out.SourceEventID != "" && out.SourceEventID != cmd.SourceEventID || out.SourceEventRevision != "" && out.SourceEventRevision != cmd.SourceEventRevision {
		return ReadResult{}, errResponse
	}
	for f, v := range out.Values {
		val, ok := scalar(v)
		if !slices.Contains(cmd.Fields, f) || !ok {
			return ReadResult{}, errResponse
		}
		out.Values[f] = val
	}
	encoded, err := canonicalBytes(out.Values)
	if err != nil || len(encoded) > 32<<10 {
		return ReadResult{}, errResponse
	}
	out.ObservationOwner = "zhuzhao"
	out.DataOwner = "activelist"
	out.SourceEventID = cmd.SourceEventID
	out.SourceEventRevision = cmd.SourceEventRevision
	return out, nil
}

func (cl *Client) Update(ctx context.Context, cmd Command) (UpdateResult, error) {
	return cl.effectResult(ctx, cmd, updatePath)
}

// LookupUpdate reconciles a submitted operation under its current grant without issuing a mutation.
func (cl *Client) LookupUpdate(ctx context.Context, cmd Command) (UpdateResult, error) {
	return cl.effectResult(ctx, cmd, receiptPath)
}

func (cl *Client) effectResult(ctx context.Context, cmd Command, path string) (UpdateResult, error) {
	var result struct {
		TypeName         string `json:"type_name"`
		DocumentID       string `json:"document_id"`
		Generation       string `json:"generation"`
		DocumentRevision string `json:"document_revision"`
		Noop             *bool  `json:"noop"`
	}
	if err := cl.post(ctx, cmd, path, &result); err != nil {
		return UpdateResult{}, err
	}
	if result.TypeName != cmd.TypeName || result.Generation != cmd.Generation || result.DocumentID != cmd.DocumentID || !b3protocol.PositiveNumber(result.DocumentRevision) || result.Noop == nil {
		return UpdateResult{}, errResponse
	}
	old, _ := strconv.ParseInt(cmd.ExpectedRevision, 10, 64)
	current, _ := strconv.ParseInt(result.DocumentRevision, 10, 64)
	if (*result.Noop && current != old) || (!*result.Noop && current <= old) {
		return UpdateResult{}, errResponse
	}
	return UpdateResult{TypeName: result.TypeName, Generation: result.Generation, DocumentID: result.DocumentID, DocumentRevision: result.DocumentRevision, Noop: *result.Noop}, nil
}

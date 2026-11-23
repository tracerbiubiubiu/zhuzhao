// Package riskasset provides a read-only, single-asset directory port.
// Directory facts are evidence for human confirmation, never local ownership commands.
package riskasset

import (
	"context"
	"errors"
	"time"
)

// Ref identifies an asset by stable source identity, never by title, IP or owner name.
type Ref struct {
	Provider   string `json:"provider"`
	ExternalID string `json:"external_id"`
}

// LookupRequest requests exactly one source identity. RequestID is signed correlation data.
type LookupRequest struct {
	AssetRef  Ref    `json:"asset_ref"`
	RequestID string `json:"request_id"`
}

// Status distinguishes directory outcomes without treating failures as empty success.
type Status string

const (
	StatusOK          Status = "ok"
	StatusNotFound    Status = "not_found"
	StatusStale       Status = "stale"
	StatusTimeout     Status = "timeout"
	StatusUnavailable Status = "unavailable"
	StatusCanceled    Status = "canceled"
)

var (
	ErrInvalidConfig  = errors.New("riskasset: invalid configuration")
	ErrInvalidRequest = errors.New("riskasset: invalid request")
	ErrUnavailable    = errors.New("riskasset: directory unavailable")
)

// Component is display evidence; this package does not infer vulnerability applicability.
type Component struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// Snapshot contains only the allowed display fields, not arbitrary provider payloads.
type Snapshot struct {
	DisplayName string      `json:"display_name"`
	Components  []Component `json:"components"`
}

// Evidence retains source facts. Empty external references remain unresolved.
// DeepLink is untrusted display data; the client never follows it.
type Evidence struct {
	AssetRef         Ref       `json:"asset_ref"`
	ObservedAt       time.Time `json:"observed_at"`
	OwnerExternalRef string    `json:"owner_external_ref,omitempty"`
	OrgExternalRef   string    `json:"org_external_ref,omitempty"`
	DeepLink         string    `json:"deep_link,omitempty"`
	Snapshot         Snapshot  `json:"snapshot"`
}

// LookupResult carries evidence only for validated ok/stale records.
type LookupResult struct {
	Status   Status
	Evidence *Evidence
}

// Port is the minimum replaceable directory dependency; it has no write methods.
// not_found/stale are domain results (nil error); other failures have safe errors.
type Port interface {
	Lookup(context.Context, LookupRequest) (LookupResult, error)
}

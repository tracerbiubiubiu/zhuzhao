package referenceassets

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/tracerbiubiubiu/zhuzhao/internal/config"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/pkg/referencequery"
)

const (
	requestTTL = 30 * time.Second
	cursorTTL  = 5 * time.Minute
)

type Service struct {
	config   config.ReferenceAssetsConfig
	revision [32]byte
	app      *referencequery.Peer
	pool     *pgxpool.Pool
	users    *repository.UserRepo
	roles    *repository.RoleRepo
	cache    *redis.Client
	slots    chan struct{}
}

func New(cfg config.ReferenceAssetsConfig, app *referencequery.Peer, pool *pgxpool.Pool, cache *redis.Client) *Service {
	return &Service{config: cfg, revision: digest(cfg), app: app, pool: pool, users: repository.NewUserRepo(pool), roles: repository.NewRoleRepo(pool), cache: cache, slots: make(chan struct{}, 8)}
}

type pendingRead struct {
	Credential middleware.ReadCredential
	Invocation [32]byte
	Assembly   [32]byte
	Original   referencequery.QueryRequest
}

type cursorBinding struct {
	Binding [32]byte
	Remote  string
}

func (s *Service) Read(ctx context.Context, cred middleware.ReadCredential, id string, query referencequery.QueryRequest) (referencequery.QueryPage, error) {
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return referencequery.QueryPage{}, quotaExceeded
	}
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	query.QueryID, query.ContractVersion, query.SourceID = referencequery.QueryID, referencequery.Version, s.config.SourceID
	if err := referencequery.ValidateInput(query); err != nil {
		return referencequery.QueryPage{}, invalidInput
	}
	allowed, err := s.prepare(ctx, cred, query)
	if err != nil {
		return referencequery.QueryPage{}, err
	}
	if len(allowed.Parameters.OwnerOrgRefs) == 0 {
		// The timestamps describe this local decision. No source was queried.
		now := time.Now().UTC().Format(time.RFC3339Nano)
		return referencequery.QueryPage{Items: []map[string]any{}, Complete: true, SourceID: s.config.SourceID,
			ObservedAt: now, FetchedAt: now, MappingVersion: "local-scope-v1", Warnings: []string{"source_not_queried"}}, nil
	}
	inv := referencequery.Invocation{RequestRef: newReferenceToken(), Audience: referencequery.ExtensionID,
		CapabilityID: referencequery.QueryID, ContractVersion: referencequery.Version, ConfigRevision: s.config.ConfigRevision,
		RequestID: id, Method: "GET", Path: referencequery.QueryPath, Query: allowed}
	key := redisKey("admission", inv.RequestRef)
	pending := pendingRead{Credential: cred, Invocation: digest(inv), Assembly: s.revision, Original: query}
	if err := s.store(ctx, key, pending, requestTTL); err != nil {
		return referencequery.QueryPage{}, err
	}
	var page referencequery.QueryPage
	if err := s.app.Post(ctx, referencequery.AppPath, inv, &page); err != nil {
		return referencequery.QueryPage{}, err
	}
	// A successful upstream response must correspond to a real callback. Consume
	// the acknowledgement so the returned page cannot certify a different read.
	data, err := s.cache.GetDel(ctx, key+":accepted").Bytes()
	var accepted referencequery.QueryRequest
	if err != nil || json.Unmarshal(data, &accepted) != nil || referencequery.ValidatePage(accepted, page) != nil {
		return referencequery.QueryPage{}, unavailable
	}
	if page.NextCursor != "" {
		cursor := newReferenceToken()
		entry := cursorBinding{Binding: s.cursorDigest(cred, query, accepted), Remote: page.NextCursor}
		if err := s.store(ctx, redisKey("cursor", cursor), entry, cursorTTL); err != nil {
			return referencequery.QueryPage{}, err
		}
		page.NextCursor = cursor
	}
	for _, row := range page.Items {
		for field := range row {
			if !slices.Contains(query.RequestedFields, field) {
				delete(row, field)
			}
		}
	}
	return page, nil
}

func (s *Service) Admit(ctx context.Context, inv referencequery.Invocation) (referencequery.Admission, error) {
	ctx, cancel := context.WithTimeout(ctx, s.config.Timeout)
	defer cancel()
	if inv.Audience != referencequery.ExtensionID || inv.Method != "GET" || inv.Path != referencequery.QueryPath ||
		inv.CapabilityID != referencequery.QueryID || inv.ContractVersion != referencequery.Version || inv.ConfigRevision != s.config.ConfigRevision ||
		inv.Query.SourceID != s.config.SourceID || !referenceToken(inv.RequestRef) {
		return referencequery.Admission{}, denied
	}
	key := redisKey("admission", inv.RequestRef)
	// Consuming the reference once prevents replays from starting another read.
	data, err := s.cache.GetDel(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return referencequery.Admission{}, denied
	}
	if err != nil {
		return referencequery.Admission{}, unavailable
	}
	var pending pendingRead
	if json.Unmarshal(data, &pending) != nil || pending.Invocation != digest(inv) || pending.Assembly != s.revision {
		return referencequery.Admission{}, denied
	}
	query, err := s.prepare(ctx, pending.Credential, pending.Original)
	if err != nil {
		return referencequery.Admission{}, err
	}
	query.Parameters.OwnerOrgRefs = slices.DeleteFunc(query.Parameters.OwnerOrgRefs, func(org string) bool {
		return !slices.Contains(inv.Query.Parameters.OwnerOrgRefs, org)
	})
	if len(query.Parameters.OwnerOrgRefs) == 0 {
		return referencequery.Admission{}, denied
	}
	if err := s.store(ctx, key+":accepted", query, requestTTL); err != nil {
		return referencequery.Admission{}, err
	}
	return referencequery.Admission{Query: query}, nil
}

func (s *Service) prepare(ctx context.Context, cred middleware.ReadCredential, query referencequery.QueryRequest) (referencequery.QueryRequest, error) {
	orgs, err := s.currentScope(ctx, cred)
	if err != nil {
		return referencequery.QueryRequest{}, err
	}
	if len(orgs) == 0 || len(orgs) > 256 {
		return referencequery.QueryRequest{}, denied
	}
	requested := query.Parameters.OwnerOrgRefs
	allowed := query
	allowed.Parameters.OwnerOrgRefs = nil
	for _, org := range orgs {
		if len(requested) == 0 || slices.Contains(requested, org) {
			allowed.Parameters.OwnerOrgRefs = append(allowed.Parameters.OwnerOrgRefs, org)
		}
	}
	allowed.RequestedFields = slices.Clone(query.RequestedFields)
	for _, field := range []string{"owner_org_ref", "device_id"} {
		if !slices.Contains(allowed.RequestedFields, field) {
			allowed.RequestedFields = append(allowed.RequestedFields, field)
		}
	}
	if query.Cursor != "" {
		if !referenceToken(query.Cursor) {
			return referencequery.QueryRequest{}, cursorChanged
		}
		data, err := s.cache.Get(ctx, redisKey("cursor", query.Cursor)).Bytes()
		if err != nil && !errors.Is(err, redis.Nil) {
			return referencequery.QueryRequest{}, unavailable
		}
		var cursor cursorBinding
		if json.Unmarshal(data, &cursor) != nil || cursor.Binding != s.cursorDigest(cred, query, allowed) {
			return referencequery.QueryRequest{}, cursorChanged
		}
		allowed.Cursor = cursor.Remote
	}
	return allowed, nil
}

func (s *Service) currentScope(ctx context.Context, cred middleware.ReadCredential) ([]string, error) {
	if cred.UserID < 1 || cred.Epoch < 1 || cred.JTI == "" || !time.Now().Before(cred.ExpiresAt) {
		return nil, denied
	}
	var scope []string
	err := repository.WithIAMCommandGuard(ctx, s.pool, func(ctx context.Context, _ pgx.Tx) error {
		snap, err := s.users.IAMAccountSecuritySnapshot(ctx, cred.UserID)
		if err != nil {
			return err
		}
		if snap.Deleted || snap.Status != 1 || snap.SessionEpoch != cred.Epoch {
			return denied
		}
		roleCodes, err := s.roles.GetEffectiveRoleCodes(ctx, cred.UserID)
		if err != nil {
			return err
		}
		if !slices.ContainsFunc(roleCodes, func(role string) bool { return slices.Contains(s.config.ReaderRoles, role) }) {
			return denied
		}
		ids, err := repository.NewOrgRepo(s.pool).ActiveReferenceMemberships(ctx, cred.UserID)
		if err != nil {
			return err
		}
		revoked, ready := middleware.CheckReadSessionProjection(ctx, s.cache, cred, snap.ProjectionEpoch)
		if revoked || !time.Now().Before(cred.ExpiresAt) {
			return denied
		}
		if !ready {
			return unavailable
		}
		for _, id := range ids {
			scope = append(scope, strconv.FormatInt(id, 10))
		}
		return nil
	})
	var rejected *referencequery.Failure
	if err != nil && !errors.As(err, &rejected) {
		return nil, unavailable
	}
	return scope, err
}

func (s *Service) cursorDigest(cred middleware.ReadCredential, query, allowed referencequery.QueryRequest) [32]byte {
	query.Cursor = ""
	return digest(struct {
		User     int64
		Session  string
		Revision [32]byte
		Query    referencequery.QueryRequest
		Scope    []string
	}{cred.UserID, cred.JTI, s.revision, query, allowed.Parameters.OwnerOrgRefs})
}

func (s *Service) store(ctx context.Context, key string, value any, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil || s.cache.Set(ctx, key, data, ttl).Err() != nil {
		return unavailable
	}
	return nil
}

func digest(value any) [32]byte {
	// The only callers use JSON-safe DTOs and primitive value types.
	encoded, _ := json.Marshal(value)
	return sha256.Sum256(encoded)
}

func redisKey(kind, token string) string {
	sum := sha256.Sum256([]byte(token))
	return "reference-assets:v1:" + kind + ":" + hex.EncodeToString(sum[:])
}

func newReferenceToken() string {
	var random [16]byte
	rand.Read(random[:])
	return hex.EncodeToString(random[:])
}

func referenceToken(value string) bool {
	if len(value) != 32 {
		return false
	}
	_, err := hex.DecodeString(value)
	return err == nil
}

var (
	quotaExceeded = &referencequery.Failure{Status: 429, Code: "quota"}
	invalidInput  = &referencequery.Failure{Status: 400, Code: "invalid_input"}
	denied        = &referencequery.Failure{Status: 403, Code: "not_authorized"}
	unavailable   = &referencequery.Failure{Status: 503, Code: "source_unavailable"}
	cursorChanged = &referencequery.Failure{Status: 409, Code: "contract_mismatch"}
)

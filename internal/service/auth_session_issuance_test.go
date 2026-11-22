package service

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgproto3"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	redispkg "github.com/tracerbiubiubiu/zhuzhao-utils/redis"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func newPortableSessionIssuer(t *testing.T) (*AuthService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return &AuthService{
		rdb: rdb,
		jwtManager: jwtpkg.NewManager(jwtpkg.Config{
			Secret: "portable-session-test-signing-key", AccessTTL: time.Minute,
		}),
		refreshTTL: time.Hour,
	}, mr
}

// c05WireDB is a test-only PostgreSQL protocol peer for auth's concrete repo.
// It does not interpret SQL or claim transaction/integration coverage: the real
// repository scans and call order run against explicit synthetic row responses.
type c05WireDB struct {
	mu            sync.Mutex
	hash          string
	current       repository.UserSecuritySnapshot
	snapshots     []repository.UserSecuritySnapshot
	snapshotReads int
	afterUserRead func()
}

func c05WireRows(backend *pgproto3.Backend, oids []uint32, values []string) {
	fields := make([]pgproto3.FieldDescription, len(oids))
	row := make([][]byte, len(values))
	for i, oid := range oids {
		fields[i] = pgproto3.FieldDescription{Name: []byte(fmt.Sprintf("f%d", i)), DataTypeOID: oid, DataTypeSize: -1, TypeModifier: -1}
		if values[i] != "<NULL>" {
			row[i] = []byte(values[i])
		}
	}
	backend.Send(&pgproto3.RowDescription{Fields: fields})
	backend.Send(&pgproto3.DataRow{Values: row})
	backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("SELECT 1")})
}

func (db *c05WireDB) query(backend *pgproto3.Backend, query string) {
	db.mu.Lock()
	defer db.mu.Unlock()
	switch {
	case strings.Contains(query, "session_projection_epoch"):
		snap := db.current
		if db.snapshotReads < len(db.snapshots) {
			snap = db.snapshots[db.snapshotReads]
		}
		db.snapshotReads++
		c05WireRows(backend, []uint32{20, 23, 16, 16, 25, 20, 20}, []string{
			"42", strconv.Itoa(snap.Status), strconv.FormatBool(snap.Deleted), strconv.FormatBool(snap.MustChangePassword),
			snap.PasswordHash, strconv.FormatInt(snap.SessionEpoch, 10), strconv.FormatInt(snap.ProjectionEpoch, 10),
		})
	case strings.Contains(query, "FROM users WHERE"):
		c05WireRows(backend, []uint32{20, 25, 25, 25, 25, 25, 25, 25, 25, 25, 23, 16, 1184, 25, 16, 20, 20, 1184, 1184, 1184}, []string{
			"42", "portable", "E880042", "", "", db.hash, "", "", "", "", "1", "false", "<NULL>", "", "false", "1", "1", "<NULL>", "2026-11-22 00:00:00+00", "2026-11-22 00:00:00+00",
		})
		if db.afterUserRead != nil {
			db.afterUserRead()
		}
	case strings.Contains(query, "UPDATE users SET last_login_at"):
		backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("UPDATE 1")})
	case strings.Contains(query, "INSERT INTO audit_logs"):
		backend.Send(&pgproto3.CommandComplete{CommandTag: []byte("INSERT 0 1")})
	default:
		backend.Send(&pgproto3.ErrorResponse{Severity: "ERROR", Code: "XX000", Message: "unexpected synthetic auth query"})
	}
}

func newC05WireAuth(t *testing.T) (*AuthService, *miniredis.Miniredis, *c05WireDB) {
	t.Helper()
	s, mr := newPortableSessionIssuer(t)
	hash, err := crypto.HashPassword("PortablePassword#42")
	require.NoError(t, err)
	db := &c05WireDB{hash: hash, current: repository.UserSecuritySnapshot{
		ID: 42, Status: 1, PasswordHash: hash, SessionEpoch: 7, ProjectionEpoch: 7,
	}}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				backend := pgproto3.NewBackend(conn, conn)
				if _, err := backend.ReceiveStartupMessage(); err != nil {
					return
				}
				backend.Send(&pgproto3.AuthenticationOk{})
				backend.Send(&pgproto3.ParameterStatus{Name: "server_version", Value: "17.0"})
				backend.Send(&pgproto3.ParameterStatus{Name: "standard_conforming_strings", Value: "on"})
				backend.Send(&pgproto3.ParameterStatus{Name: "client_encoding", Value: "UTF8"})
				backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
				if backend.Flush() != nil {
					return
				}
				for {
					message, err := backend.Receive()
					if err != nil {
						return
					}
					switch message := message.(type) {
					case *pgproto3.Query:
						db.query(backend, message.String)
						backend.Send(&pgproto3.ReadyForQuery{TxStatus: 'I'})
						if backend.Flush() != nil {
							return
						}
					case *pgproto3.Terminate:
						return
					default:
						return
					}
				}
			}()
		}
	}()
	cfg, err := pgxpool.ParseConfig("postgres://portable@" + listener.Addr().String() + "/portable?sslmode=disable")
	require.NoError(t, err)
	cfg.ConnConfig.DefaultQueryExecMode = pgx.QueryExecModeSimpleProtocol
	cfg.MaxConns = 4
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	s.pool = pool
	s.userRepo = repository.NewUserRepo(pool)
	s.opsRepo = repository.NewSessionOperationRepo(pool)
	s.auditService = NewAuditService(repository.NewAuditLogRepo(pool), s.userRepo)
	s.scripts = redispkg.NewScripts(s.rdb)
	mr.Set("user:session_epoch:db1:42", "7")
	return s, mr, db
}

// Regressions caught: production Login emitting the Redis pwe default instead of
// its DB credential snapshot, or clearing compatibility disabled state.
func TestC05Portable_LoginUsesCredentialSnapshotEpoch(t *testing.T) {
	s, mr, _ := newC05WireAuth(t)
	mr.Set("user:pw_epoch:42", "777")
	mr.Set("user:disabled:42", "legacy")
	pair, err := s.Login(context.Background(), &model.LoginRequest{EmployeeNo: "E880042", Password: "PortablePassword#42", DeviceID: "phone"}, "127.0.0.1", "portable")
	require.NoError(t, err)
	at, err := s.jwtManager.ParseAccessToken(pair.AccessToken)
	require.NoError(t, err)
	require.Equal(t, "7", at.SessionEpoch)
	require.Zero(t, at.Pwe)
	require.True(t, mr.Exists("user:disabled:42"))
}

// Regressions caught: authenticating a reread credential but issuing the epoch
// captured from a different credential, including a hash change without epoch.
func TestC05Portable_LoginRejectsChangedCredentialSnapshot(t *testing.T) {
	for _, change := range []string{"hash", "epoch", "disabled", "deleted"} {
		t.Run(change, func(t *testing.T) {
			s, mr, db := newC05WireAuth(t)
			db.mu.Lock()
			after := db.current
			switch change {
			case "hash":
				after.PasswordHash = "different-credential"
			case "epoch":
				after.SessionEpoch = 8
				after.ProjectionEpoch = 8
				mr.Set("user:session_epoch:db1:42", "8")
			case "disabled":
				after.Status = 0
			case "deleted":
				after.Deleted = true
			}
			db.snapshots = []repository.UserSecuritySnapshot{db.current, after}
			db.mu.Unlock()
			pair, err := s.Login(context.Background(), &model.LoginRequest{EmployeeNo: "E880042", Password: "PortablePassword#42", DeviceID: "phone"}, "127.0.0.1", "portable")
			require.ErrorIs(t, err, errcode.ErrInvalidCredentials)
			require.Nil(t, pair)
			require.False(t, mr.Exists("refresh:db1:42:7:phone"))
			require.False(t, mr.Exists("refresh:db1:42:8:phone"))
		})
	}
}

func signC05Refresh(t *testing.T, s *AuthService, schema, epoch string) string {
	t.Helper()
	raw, err := jwt.NewWithClaims(jwt.SigningMethodHS256, &jwtpkg.RefreshClaims{
		UserID: 42, DeviceID: "phone", TokenType: jwtpkg.TokenTypeRefresh, SessionSchema: schema, SessionEpoch: epoch,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}).SignedString([]byte("portable-session-test-signing-key"))
	require.NoError(t, err)
	return raw
}

// Regressions caught: legacy/unknown schema or noncanonical epochs reaching a
// valid RT slot through int parsing/defaults rather than the signed contract.
func TestC05Portable_RefreshRejectsMalformedSessionClaims(t *testing.T) {
	for _, tc := range []struct{ schema, epoch string }{
		{"", ""}, {"unknown", "7"}, {"db_epoch_v1", ""}, {"db_epoch_v1", "0"},
		{"db_epoch_v1", "-1"}, {"db_epoch_v1", "+7"}, {"db_epoch_v1", "07"},
		{"db_epoch_v1", "7.0"}, {"db_epoch_v1", "9223372036854775808"},
	} {
		t.Run(tc.schema+"/"+tc.epoch, func(t *testing.T) {
			s, mr, _ := newC05WireAuth(t)
			raw := signC05Refresh(t, s, tc.schema, tc.epoch)
			mr.Set("refresh:42:phone", hashToken(raw))
			mr.Set("refresh:db1:42:7:phone", "current-session")
			pair, err := s.Refresh(context.Background(), raw, "127.0.0.1", "portable")
			require.ErrorIs(t, err, errcode.ErrRefreshTokenInvalid)
			require.Nil(t, pair)
			stored, err := mr.Get("refresh:db1:42:7:phone")
			require.NoError(t, err)
			require.Equal(t, "current-session", stored)
		})
	}
}

// Regressions caught: GETDEL followed by SET allowing two refreshes to win, or
// a stale hash deleting the newer RT installed by Login on the same device.
func TestC05Portable_RefreshCASOneWinnerAndPreservesNewerSlot(t *testing.T) {
	s, mr, _ := newC05WireAuth(t)
	old, _, err := s.jwtManager.GenerateRefreshTokenWithSessionEpoch(42, "phone", time.Hour, 0, 7)
	require.NoError(t, err)
	mr.Set("refresh:db1:42:7:phone", hashToken(old))
	const contenders = 6
	var wg sync.WaitGroup
	pairs := make(chan *model.TokenPair, contenders)
	for range contenders {
		wg.Add(1)
		go func() {
			defer wg.Done()
			pair, _ := s.Refresh(context.Background(), old, "127.0.0.1", "portable")
			if pair != nil {
				pairs <- pair
			}
		}()
	}
	wg.Wait()
	close(pairs)
	var winners []*model.TokenPair
	for pair := range pairs {
		winners = append(winners, pair)
	}
	require.Len(t, winners, 1)
	stored, err := mr.Get("refresh:db1:42:7:phone")
	require.NoError(t, err)
	require.Equal(t, hashToken(winners[0].RefreshToken), stored)
	// Replaying the old token must leave the winner usable.
	pair, err := s.Refresh(context.Background(), old, "127.0.0.1", "portable")
	require.ErrorIs(t, err, errcode.ErrRefreshTokenReplayed)
	require.Nil(t, pair)
	again, err := mr.Get("refresh:db1:42:7:phone")
	require.NoError(t, err)
	require.Equal(t, stored, again)
	_, err = s.Refresh(context.Background(), winners[0].RefreshToken, "127.0.0.1", "portable")
	require.NoError(t, err)
}

// Regression caught: Logout deleting an unversioned slot or a newer epoch slot.
func TestC05Portable_LogoutDeletesOnlyOriginalEpochSlot(t *testing.T) {
	s, mr := newPortableSessionIssuer(t)
	at, _, err := s.jwtManager.GenerateAccessTokenWithSessionEpoch(42, "portable", false, 0, 7)
	require.NoError(t, err)
	mr.Set("refresh:db1:42:7:phone", "original")
	mr.Set("refresh:db1:42:8:phone", "new-login")
	mr.Set("refresh:42:phone", "legacy")
	require.NoError(t, s.Logout(context.Background(), at, "phone"))
	require.False(t, mr.Exists("refresh:db1:42:7:phone"))
	newer, err := mr.Get("refresh:db1:42:8:phone")
	require.NoError(t, err)
	require.Equal(t, "new-login", newer)
	legacy, err := mr.Get("refresh:42:phone")
	require.NoError(t, err)
	require.Equal(t, "legacy", legacy)
}

func TestC05Portable_RefreshPreservesInterleavedNewLogin(t *testing.T) {
	s, mr, db := newC05WireAuth(t)
	old, _, err := s.jwtManager.GenerateRefreshTokenWithSessionEpoch(42, "phone", time.Hour, 0, 7)
	require.NoError(t, err)
	mr.Set("refresh:db1:42:7:phone", hashToken(old))
	var login *model.TokenPair
	var issueErr error
	db.mu.Lock()
	db.afterUserRead = func() {
		login, issueErr = s.issueTokenPair(context.Background(), &model.User{ID: 42, Username: "portable"}, "phone", 7)
	}
	db.mu.Unlock()
	pair, err := s.Refresh(context.Background(), old, "127.0.0.1", "portable")
	require.ErrorIs(t, err, errcode.ErrRefreshTokenReplayed)
	require.Nil(t, pair)
	db.mu.Lock()
	db.afterUserRead = nil
	issued := login
	issuedErr := issueErr
	db.mu.Unlock()
	require.NoError(t, issuedErr)
	require.NotNil(t, issued)
	stored, err := mr.Get("refresh:db1:42:7:phone")
	require.NoError(t, err)
	require.Equal(t, hashToken(issued.RefreshToken), stored)
	_, err = s.Refresh(context.Background(), issued.RefreshToken, "127.0.0.1", "portable")
	require.NoError(t, err)
}

func TestC05Portable_RefreshRejectsDBFactsAndProjectionLag(t *testing.T) {
	for _, tc := range []struct {
		name   string
		change func(*c05WireDB, *miniredis.Miniredis)
		want   error
	}{
		{"disabled", func(db *c05WireDB, _ *miniredis.Miniredis) { db.current.Status = 0 }, errcode.ErrRefreshTokenInvalid},
		{"deleted", func(db *c05WireDB, _ *miniredis.Miniredis) { db.current.Deleted = true }, errcode.ErrRefreshTokenInvalid},
		{"old-epoch", func(db *c05WireDB, _ *miniredis.Miniredis) { db.current.SessionEpoch = 8 }, errcode.ErrPasswordChanged},
		{"unacknowledged", func(db *c05WireDB, _ *miniredis.Miniredis) { db.current.ProjectionEpoch = 6 }, errcode.ErrServiceUnavailable},
		{"missing-projection", func(_ *c05WireDB, mr *miniredis.Miniredis) { mr.Del("user:session_epoch:db1:42") }, errcode.ErrServiceUnavailable},
		{"wrong-projection", func(_ *c05WireDB, mr *miniredis.Miniredis) { mr.Set("user:session_epoch:db1:42", "8") }, errcode.ErrServiceUnavailable},
		{"redis-disabled", func(_ *c05WireDB, mr *miniredis.Miniredis) { mr.Set("user:session_disabled:db1:42", "1") }, errcode.ErrServiceUnavailable},
		{"projection-advances-before-cas", func(db *c05WireDB, mr *miniredis.Miniredis) {
			db.afterUserRead = func() { mr.Set("user:session_epoch:db1:42", "8") }
		}, errcode.ErrServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mr, db := newC05WireAuth(t)
			old, _, err := s.jwtManager.GenerateRefreshTokenWithSessionEpoch(42, "phone", time.Hour, 0, 7)
			require.NoError(t, err)
			mr.Set("refresh:db1:42:7:phone", hashToken(old))
			db.mu.Lock()
			tc.change(db, mr)
			db.mu.Unlock()
			pair, err := s.Refresh(context.Background(), old, "127.0.0.1", "portable")
			require.ErrorIs(t, err, tc.want)
			require.Nil(t, pair)
			stored, err := mr.Get("refresh:db1:42:7:phone")
			require.NoError(t, err)
			require.Equal(t, hashToken(old), stored)
		})
	}
}

// Regressions caught: signing legacy pwe tokens or storing an unversioned RT slot.
func TestC05Portable_IssuesDBEpochPairAndVersionedSlot(t *testing.T) {
	s, mr := newPortableSessionIssuer(t)
	mr.Set("user:session_epoch:db1:42", "9007199254740993")
	// Legacy state must not affect issuance or be mutated by the new protocol.
	mr.Set("user:pw_epoch:42", "777")
	mr.Set("refresh:42:phone", "legacy-slot")
	pair, err := s.issueTokenPair(context.Background(), &model.User{ID: 42, Username: "portable", MustChangePassword: true}, "phone", 9007199254740993)
	require.NoError(t, err)
	at, err := s.jwtManager.ParseAccessToken(pair.AccessToken)
	require.NoError(t, err)
	rt, err := s.jwtManager.ParseRefreshToken(pair.RefreshToken)
	require.NoError(t, err)
	require.Equal(t, "db_epoch_v1", at.SessionSchema)
	require.Equal(t, "db_epoch_v1", rt.SessionSchema)
	require.Equal(t, "9007199254740993", at.SessionEpoch)
	require.Equal(t, "9007199254740993", rt.SessionEpoch)
	require.Zero(t, at.Pwe)
	require.Zero(t, rt.Pwe)
	require.True(t, at.MustChangePassword)
	stored, err := mr.Get("refresh:db1:42:9007199254740993:phone")
	require.NoError(t, err)
	require.Equal(t, hashToken(pair.RefreshToken), stored)
	legacy, err := mr.Get("refresh:42:phone")
	require.NoError(t, err)
	require.Equal(t, "legacy-slot", legacy)
}

// Regressions caught: silently defaulting a missing projection or borrowing its newer epoch.
func TestC05Portable_IssuanceRequiresExactActiveProjection(t *testing.T) {
	for _, tc := range []struct {
		name, projected string
		disabled        bool
	}{
		{name: "missing"},
		{name: "legacy-zero", projected: "0"},
		{name: "newer", projected: "8"},
		{name: "older", projected: "6"},
		{name: "noncanonical", projected: "07"},
		{name: "disabled", projected: "7", disabled: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, mr := newPortableSessionIssuer(t)
			if tc.projected != "" {
				mr.Set("user:session_epoch:db1:42", tc.projected)
			}
			if tc.disabled {
				mr.Set("user:session_disabled:db1:42", "1")
			}
			pair, err := s.issueTokenPair(context.Background(), &model.User{ID: 42}, "phone", 7)
			require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
			require.Nil(t, pair)
			require.False(t, mr.Exists("refresh:db1:42:7:phone"))
		})
	}
}

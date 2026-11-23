//go:build integration

package riskremediation

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

var remediationTestPool *pgxpool.Pool

func TestMain(m *testing.M) {
	pool, cleanup, err := testutil.SetupPostgresShared()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	remediationTestPool = pool
	code := m.Run()
	cleanup()
	os.Exit(code)
}

// Each test owns a schema in the package's disposable PostgreSQL container.
// The synthetic foundational tables exercise persisted risk facts and callbacks.
type pgFixture struct {
	t      *testing.T
	ctx    context.Context
	pool   *pgxpool.Pool
	svc    *Service
	ticket int64
	risks  []int64
	links  []int64
}

const (
	fixtureOwner   int64 = 101
	fixtureHandler int64 = 202
	fixtureManager int64 = 303
	fixtureOther   int64 = 404
)

func newPGFixture(t *testing.T, riskCount int) *pgFixture {
	t.Helper()
	dsn := remediationTestPool.Config().ConnString()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	admin := remediationTestPool
	var version string
	require.NoError(t, admin.QueryRow(ctx, "SHOW server_version_num").Scan(&version))
	v, err := strconv.Atoi(version)
	require.NoError(t, err)
	require.True(t, v >= 180000 && v < 190000, "exclusive PostgreSQL 18 is required")
	schema := "risk_remediation_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	_, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize())
	require.NoError(t, err)
	t.Cleanup(func() {
		cleanup, stop := context.WithTimeout(context.Background(), 30*time.Second)
		defer stop()
		_, cleanupErr := admin.Exec(cleanup, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		require.NoError(t, cleanupErr)
	})
	cfg, err := pgxpool.ParseConfig(dsn)
	require.NoError(t, err)
	cfg.MaxConns = 8
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog"
	cfg.ConnConfig.Tracer = &fixtureQueryTrace{t: t}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	_, err = pool.Exec(ctx, fixtureFoundationSQL)
	require.NoError(t, err)
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	root := filepath.Clean(filepath.Join(filepath.Dir(currentFile), "../../.."))
	for _, name := range []string{"000089_risk_registration.up.sql", "000090_risk_personal_dispatch.up.sql", "000092_risk_remediation.up.sql", "000095_risk_retest_handoff.up.sql"} {
		migration, readErr := os.ReadFile(filepath.Join(root, "migrations", name))
		require.NoError(t, readErr, "required original migration %s", name)
		_, err = pool.Exec(ctx, string(migration))
		require.NoError(t, err, "apply original migration %s", name)
	}
	f := &pgFixture{t: t, ctx: ctx, pool: pool}
	f.svc = New(pool, f.callbacks())
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO tickets(type_code,status,created_by,assigned_to,org_id,revision) VALUES('remediation','in_progress',$1,$2,1,1) RETURNING id`, fixtureOwner, fixtureHandler).Scan(&f.ticket))
	var group int64
	require.NoError(t, pool.QueryRow(ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id) VALUES('SYNTHETIC-CVE', $1,1,1,'remediation',1,'synthetic-only',repeat('a',64),$2,'SYNTHETIC-TICKET',$3) RETURNING id`, fixtureOwner, f.ticket, fixtureManager).Scan(&group))
	for i := 0; i < riskCount; i++ {
		var family, risk, link int64
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('synthetic',$1) RETURNING id`, fmt.Sprintf("asset-%d", i)).Scan(&family))
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,treatment,title,created_by) VALUES($1,$2,'remediating',1,$3,'mitigate',$1,$4) RETURNING id`, fmt.Sprintf("SYNTHETIC-%d", i), family, fixtureOwner, fixtureManager).Scan(&risk))
		require.NoError(t, pool.QueryRow(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3) RETURNING id`, risk, group, fixtureOwner).Scan(&link))
		f.risks = append(f.risks, risk)
		f.links = append(f.links, link)
	}
	return f
}

func TestRetestHandoffMigrationRoundtripUnused(t *testing.T) {
	f := newPGFixture(t, 0)
	_, currentFile, _, ok := runtime.Caller(0)
	require.True(t, ok)
	tx, err := f.pool.Begin(f.ctx)
	require.NoError(t, err)
	defer tx.Rollback(f.ctx)
	for _, migration := range []string{"000095_risk_retest_handoff.down.sql", "000095_risk_retest_handoff.up.sql"} {
		contents, err := os.ReadFile(filepath.Join(filepath.Dir(currentFile), "../../../migrations", migration))
		require.NoError(t, err)
		_, err = tx.Exec(f.ctx, string(contents))
		require.NoError(t, err, migration)
	}
	require.NoError(t, tx.Commit(f.ctx))
}

// Only this disposable synthetic database is traced. Failure diagnostics are
// retained in test output without weakening production storage-error mapping.
type fixtureQueryTrace struct{ t *testing.T }

func (*fixtureQueryTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	return ctx
}
func (trace *fixtureQueryTrace) TraceQueryEnd(_ context.Context, _ *pgx.Conn, data pgx.TraceQueryEndData) {
	if data.Err != nil {
		trace.t.Logf("synthetic PostgreSQL failure: %v", data.Err)
	}
}

// The adapters are intentionally synthetic and SQL-backed. Their only purpose
// is to expose owner/revocation checks and all ticket writes on the supplied Tx.
func (f *pgFixture) callbacks() Callbacks {
	check := func(ctx context.Context, tx pgx.Tx, a Access) error {
		var enabled, manage, anchor bool
		if err := tx.QueryRow(ctx, `SELECT enabled,manage,main_anchor FROM fixture_actors WHERE id=$1`, a.ActorID).Scan(&enabled, &manage, &anchor); err != nil {
			return errcode.ErrNoPermission
		}
		if !enabled || !manage {
			return errcode.ErrNoPermission
		}
		if strings.Contains(a.Action, "review") && a.ActorID != a.OwnerID {
			return errcode.ErrNoPermission
		}
		if strings.Contains(a.Action, "accept") && strings.Contains(a.Action, "request") && a.ActorID != a.OwnerID {
			return errcode.ErrNoPermission
		}
		if strings.Contains(a.Action, "accept") && (strings.Contains(a.Action, "decid") || strings.Contains(a.Action, "confirm")) && !anchor {
			return errcode.ErrNoPermission
		}
		return nil
	}
	return Callbacks{
		Admit: func(ctx context.Context, tx pgx.Tx, a Access) error {
			var enabled bool
			if err := tx.QueryRow(ctx, `SELECT enabled FROM fixture_actors WHERE id=$1`, a.ActorID).Scan(&enabled); err != nil || !enabled {
				return errcode.ErrNoPermission
			}
			return nil
		},
		// This synthetic organization/definition adapter deliberately acquires
		// no domain locks; real adapters must honor the kernel lock order.
		Prelock:   func(context.Context, pgx.Tx, Access) error { return nil },
		Authorize: check,
		TicketPolicy: func(ctx context.Context, tx pgx.Tx, c TicketCheck) (Policy, error) {
			var revision int64
			if err := tx.QueryRow(ctx, `SELECT revision FROM tickets WHERE id=$1`, c.TicketID).Scan(&revision); err != nil {
				return Policy{}, err
			}
			if revision != c.ExpectedRevision {
				return Policy{}, errcode.ErrConflict
			}
			return Policy{ReviewGate: true}, nil
		},
		Evidence: func(_ context.Context, _ pgx.Tx, c EvidenceCheck) error {
			for _, ref := range c.Evidence {
				if ref.Kind != "reference" || ref.Ref == "" || ref.Version != "1" {
					return errcode.ErrInvalidParams
				}
			}
			return nil
		},
		ApplyTicket: func(ctx context.Context, tx pgx.Tx, c TicketEffect) (int64, error) {
			status := "pending_verify"
			if c.TerminalAllowed {
				status = "closed"
			}
			var revision int64
			if err := tx.QueryRow(ctx, `UPDATE tickets SET status=$2,revision=revision+1 WHERE id=$1 AND revision=$3 RETURNING revision`, c.TicketID, status, c.ExpectedRevision).Scan(&revision); err != nil {
				return 0, err
			}
			if _, err := tx.Exec(ctx, `INSERT INTO fixture_ticket_events(ticket_id,action) VALUES($1,$2)`, c.TicketID, c.Action); err != nil {
				return 0, err
			}
			_, err := tx.Exec(ctx, `INSERT INTO fixture_ticket_intents(ticket_id,action) VALUES($1,$2)`, c.TicketID, c.Action)
			return revision, err
		},
	}
}

func (f *pgFixture) guarded(fn func(context.Context, pgx.Tx) error) error {
	return repository.WithIAMCommandGuard(f.ctx, f.pool, fn)
}

func (f *pgFixture) command(actor int64, operation string) Command {
	f.t.Helper()
	var revision int64
	require.NoError(f.t, f.pool.QueryRow(f.ctx, `SELECT revision FROM tickets WHERE id=$1`, f.ticket).Scan(&revision))
	return Command{ActorID: actor, OperationID: operation, TicketID: f.ticket, ExpectedTicketRevision: revision}
}

func (f *pgFixture) item(index int) Item {
	f.t.Helper()
	var linkRevision, riskRevision int64
	require.NoError(f.t, f.pool.QueryRow(f.ctx, `SELECT l.revision,r.revision FROM risk_ticket_links l JOIN risk_items r ON r.id=l.risk_id WHERE l.id=$1`, f.links[index]).Scan(&linkRevision, &riskRevision))
	return Item{LinkID: f.links[index], ExpectedLinkRevision: linkRevision, ExpectedRiskRevision: riskRevision, Resolution: "fixed", Measures: "synthetic patch applied", Evidence: []EvidenceRef{{Kind: "reference", Ref: fmt.Sprintf("synthetic-asset-%d", index), Version: "1"}}}
}

func (f *pgFixture) finish(c FinishCommand) (result Result, err error) {
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		result, err = f.svc.FinishTx(ctx, tx, c)
		return err
	})
	return result, err
}

func (f *pgFixture) review(c ReviewCommand) (result Result, err error) {
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		result, err = f.svc.ReviewTx(ctx, tx, c)
		return err
	})
	return result, err
}

func (f *pgFixture) requestAcceptance(c AcceptanceRequest) (result Result, err error) {
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		result, err = f.svc.RequestAcceptanceTx(ctx, tx, c)
		return err
	})
	return result, err
}

func (f *pgFixture) decideAcceptance(c AcceptanceDecision) (result Result, err error) {
	err = f.guarded(func(ctx context.Context, tx pgx.Tx) error {
		result, err = f.svc.DecideAcceptanceTx(ctx, tx, c)
		return err
	})
	return result, err
}

// Sequence advancement is deliberately excluded: PostgreSQL sequences are not
// transactional. Every business fact, history, receipt and durable intent is.
func (f *pgFixture) snapshot() string {
	f.t.Helper()
	var snapshot string
	require.NoError(f.t, f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object(
	'risks',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM risk_items t),
	'links',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM risk_ticket_links t),
	'tickets',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM tickets t),
	'submissions',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM risk_remediation_submissions t),
	'reviews',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM risk_remediation_reviews t),
	'acceptances',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM risk_acceptances t),
	'events',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM risk_events t),
	'operations',(SELECT jsonb_agg(to_jsonb(t) ORDER BY actor_id,operation_id) FROM risk_operations t),
	'receipts',(SELECT jsonb_agg(to_jsonb(t) ORDER BY actor_id,operation_id) FROM risk_remediation_receipts t),
	'intents',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM risk_remediation_intents t),
	'ticket_events',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM fixture_ticket_events t),
	'ticket_intents',(SELECT jsonb_agg(to_jsonb(t) ORDER BY id) FROM fixture_ticket_intents t)
	)::text`).Scan(&snapshot))
	return snapshot
}

func (f *pgFixture) riskLinkSnapshot(index int) string {
	f.t.Helper()
	var snapshot string
	require.NoError(f.t, f.pool.QueryRow(f.ctx, `SELECT jsonb_build_object('risk',to_jsonb(r),'link',to_jsonb(l))::text FROM risk_items r JOIN risk_ticket_links l ON l.risk_id=r.id WHERE l.id=$1`, f.links[index]).Scan(&snapshot))
	return snapshot
}

func (f *pgFixture) factsSnapshot() string {
	f.t.Helper()
	var facts string
	require.NoError(f.t, f.pool.QueryRow(f.ctx, `SELECT ($1::jsonb-'operations'-'receipts')::text`, f.snapshot()).Scan(&facts))
	return facts
}

func (f *pgFixture) reviewItem(index int, decision string) ReviewItem {
	f.t.Helper()
	i := f.item(index)
	var proposal int64
	require.NoError(f.t, f.pool.QueryRow(f.ctx, `SELECT proposal_version FROM risk_ticket_links WHERE id=$1`, i.LinkID).Scan(&proposal))
	return ReviewItem{LinkID: i.LinkID, ExpectedLinkRevision: i.ExpectedLinkRevision, ExpectedRiskRevision: i.ExpectedRiskRevision, ExpectedProposalVersion: proposal, Decision: decision, Note: "synthetic review reason", Evidence: i.Evidence}
}

const fixtureFoundationSQL = `
CREATE TABLE organizations(id BIGINT PRIMARY KEY);
INSERT INTO organizations VALUES(1);
CREATE TABLE ticket_types(code VARCHAR(50) PRIMARY KEY);
INSERT INTO ticket_types VALUES('remediation');
CREATE TABLE tickets(id BIGSERIAL PRIMARY KEY,type_code VARCHAR(50) NOT NULL REFERENCES ticket_types(code),status TEXT NOT NULL,created_by BIGINT NOT NULL,assigned_to BIGINT,org_id BIGINT NOT NULL REFERENCES organizations(id),revision BIGINT NOT NULL DEFAULT 1);
CREATE TABLE menus(id BIGSERIAL PRIMARY KEY,parent_id BIGINT,code TEXT UNIQUE,name TEXT,menu_type INT,path TEXT,component TEXT,icon TEXT,permission TEXT,sort_order INT,is_system BOOLEAN,deleted_at TIMESTAMPTZ);
CREATE TABLE roles(id BIGSERIAL PRIMARY KEY,code TEXT UNIQUE,deleted_at TIMESTAMPTZ);
INSERT INTO roles(code) VALUES('admin'),('superadmin');
CREATE TABLE menu_apis(menu_id BIGINT REFERENCES menus(id),api_path TEXT,api_method TEXT,PRIMARY KEY(menu_id,api_path,api_method));
CREATE TABLE role_menus(role_id BIGINT REFERENCES roles(id),menu_id BIGINT REFERENCES menus(id),PRIMARY KEY(role_id,menu_id));
CREATE TABLE casbin_rule(id BIGSERIAL PRIMARY KEY,ptype TEXT,v0 TEXT,v1 TEXT,v2 TEXT,UNIQUE(ptype,v0,v1,v2));
CREATE TABLE fixture_actors(id BIGINT PRIMARY KEY,enabled BOOLEAN NOT NULL,manage BOOLEAN NOT NULL,main_anchor BOOLEAN NOT NULL);
INSERT INTO fixture_actors VALUES(101,true,true,false),(202,true,true,false),(303,true,true,true),(404,true,true,false);
CREATE TABLE fixture_ticket_events(id BIGSERIAL PRIMARY KEY,ticket_id BIGINT REFERENCES tickets(id) ON DELETE SET NULL,action TEXT);
CREATE TABLE fixture_ticket_intents(id BIGSERIAL PRIMARY KEY,ticket_id BIGINT REFERENCES tickets(id) ON DELETE SET NULL,action TEXT);
`

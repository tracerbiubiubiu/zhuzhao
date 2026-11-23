//go:build integration

package riskreminder

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const eligibilityModel = `[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.sub == p.sub && keyMatch2(r.obj, p.obj) && r.act == p.act
`

type eligibilityFixture struct {
	*fixture
	policy AcceptancePolicy
	role   int64
	code   string
}

// The fixture supplies an explicit synthetic policy; node SLA metadata is never
// an authority to publish a first-acceptance reminder policy.
func newEligibilityFixture(t *testing.T) *eligibilityFixture {
	t.Helper()
	f := &eligibilityFixture{fixture: createFixture(t), code: "eligibility_" + randSuffix()}
	ctx := context.Background()
	snapshot, err := json.Marshal(map[string]any{
		"type": map[string]any{
			"code": f.TypeCode, "name": "synthetic eligibility",
			"states":          []string{"open", "assigned", "in_progress", "closed"},
			"transitions":     map[string][]string{"open": {"assigned"}, "assigned": {"in_progress"}, "in_progress": {"closed"}},
			"transition_meta": map[string]any{}, "default_sla_hours": 48, "has_custom_fields": false,
		}, "fields": []any{},
	})
	require.NoError(t, err)
	ref := model.TicketDefinitionRef{TypeCode: f.TypeCode, DefinitionVersion: 1}
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by)
		VALUES($1,1,ticket_definition_snapshot_hash($2::jsonb),1,$2,1,$3) RETURNING definition_hash`, f.TypeCode, snapshot, f.M).Scan(&ref.DefinitionHash))
	_, err = testPool.Exec(ctx, `UPDATE ticket_types SET current_published_version=1 WHERE code=$1`, f.TypeCode)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE tickets SET assigned_to=$2,status='assigned',definition_version=1 WHERE id=$1`, f.TicketID, f.P)
	require.NoError(t, err)
	addVisibleRiskLink(t, f.fixture, f.code)
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority) VALUES($1,'synthetic eligibility role',100) RETURNING id`, f.code).Scan(&f.role))
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, f.P, f.role)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,'/api/v1/tickets','GET'),('p',$1,'/api/v1/tickets/:id','GET'),('p',$1,'/api/v1/tickets/transition','POST'),('p',$1,'/api/v1/risk-items/remediation-accept-assignment','POST')`, "role::"+f.code)
	require.NoError(t, err)
	f.policy = AcceptancePolicy{DefinitionRef: ref, RuleID: "synthetic-first-accept", RuleVersion: 1, AwaitingState: "assigned", AcceptWithin: 30 * time.Minute}
	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, f.TicketID); err != nil {
			return err
		}
		_, err := RecordAssignmentTx(ctx, tx, &AssignmentInput{TicketID: f.TicketID, Generation: 1, AssigneeID: f.P, DispatchedBy: f.M, DispatchedAt: tDispatch, AcceptDueAt: tDue, RuleID: f.policy.RuleID, RuleVersion: 1})
		return err
	}))
	return f
}

func addVisibleRiskLink(t *testing.T, f *fixture, key string) {
	t.Helper()
	ctx := context.Background()
	var risk, family, group int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope,canonical_key_hash,key_schema_version)
		VALUES('synthetic-reminder',$1,1) RETURNING id`, key).Scan(&family))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,org_id,owner_user_id,title,created_by)
		VALUES($1,$2,$3,$4,'synthetic reminder risk',$4) RETURNING id`, key, family, f.OrgID, f.M).Scan(&risk))
	_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, f.P, f.OrgID)
	require.NoError(t, err)
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,
		target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id)
		VALUES($1,$2,$3,$3,$4,1,'synthetic',repeat('a',64),$5,$6,$2) RETURNING id`,
		key, f.M, f.OrgID, f.TypeCode, f.TicketID, f.HumanKey).Scan(&group))
	_, err = testPool.Exec(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3)`, risk, group, f.M)
	require.NoError(t, err)
}
func (f *eligibilityFixture) resolver(context.Context, pgx.Tx, int64) (*AcceptancePolicy, error) {
	p := f.policy
	return &p, nil
}

func checkEligibility(t *testing.T, pool *pgxpool.Pool, f *eligibilityFixture, checker EligibilityChecker) (eligible bool, reason string, checkErr error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, repository.WithIAMCommandGuard(ctx, pool, func(ctx context.Context, tx pgx.Tx) error {
		eligible, reason, checkErr = checker(ctx, tx, f.TicketID, f.P)
		return nil
	}))
	return
}

// Losing any route or its live role must stop delivery even if a previous
// eligibility check succeeded; an in-memory/cached Casbin grant is insufficient.
func TestEligibilityCurrentAuthorization(t *testing.T) {
	for _, mutation := range []string{
		`DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets'`,
		`DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets/:id'`,
		`DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/risk-items/remediation-accept-assignment'`,
		`UPDATE roles SET status=0 WHERE code=substring($1 FROM 7)`,
	} {
		t.Run(mutation, func(t *testing.T) {
			f := newEligibilityFixture(t)
			checker := NewEligibilityChecker(testPool, eligibilityModel, f.resolver)
			ok, reason, err := checkEligibility(t, testPool, f, checker)
			require.NoError(t, err)
			require.True(t, ok)
			require.Empty(t, reason)
			require.NoError(t, repository.WithIAMGuard(context.Background(), testPool, func(ctx context.Context) error {
				_, err := repository.IAMDB(ctx, testPool).Exec(ctx, mutation, "role::"+f.code)
				return err
			}))
			ok, reason, err = checkEligibility(t, testPool, f, checker)
			require.NoError(t, err)
			require.False(t, ok)
			require.Equal(t, "recipient_unauthorized", reason)
		})
	}
}

func TestEligibilityRequiresCurrentRiskVisibility(t *testing.T) {
	f := newEligibilityFixture(t)
	checker := NewEligibilityChecker(testPool, eligibilityModel, f.resolver)
	allowed, _, err := checkEligibility(t, testPool, f, checker)
	require.NoError(t, err)
	require.True(t, allowed)
	require.NoError(t, repository.WithIAMGuard(context.Background(), testPool, func(ctx context.Context) error {
		_, err := repository.IAMDB(ctx, testPool).Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1 AND org_id=$2`, f.P, f.OrgID)
		return err
	}))
	allowed, reason, err := checkEligibility(t, testPool, f, checker)
	require.NoError(t, err)
	require.False(t, allowed)
	require.Equal(t, "recipient_unauthorized", reason)
}

func TestEligibilityCurrentAccountAndTicketFacts(t *testing.T) {
	for _, tc := range []struct{ name, sql, reason string }{
		{"disabled", `UPDATE users SET status=0 WHERE id=$1`, "recipient_disabled"},
		{"deleted", `UPDATE users SET deleted_at=now() WHERE id=$1`, "recipient_disabled"},
		{"reassigned", `UPDATE tickets SET assigned_to=NULL WHERE id=$1`, "assignment_changed"},
		{"accepted", `UPDATE tickets SET status='in_progress' WHERE id=$1`, "not_awaiting_acceptance"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newEligibilityFixture(t)
			checker := NewEligibilityChecker(testPool, eligibilityModel, f.resolver)
			id := f.TicketID
			if tc.name == "disabled" || tc.name == "deleted" {
				id = f.P
			}
			require.NoError(t, repository.WithIAMGuard(context.Background(), testPool, func(ctx context.Context) error {
				_, err := repository.IAMDB(ctx, testPool).Exec(ctx, tc.sql, id)
				return err
			}))
			ok, reason, err := checkEligibility(t, testPool, f, checker)
			require.NoError(t, err)
			require.False(t, ok)
			require.Equal(t, tc.reason, reason)
		})
	}
}

func TestEligibilityRequiresPublishedPolicy(t *testing.T) {
	f := newEligibilityFixture(t)
	for _, resolver := range []PolicyResolver{nil, func(context.Context, pgx.Tx, int64) (*AcceptancePolicy, error) { return nil, nil }} {
		ok, reason, err := checkEligibility(t, testPool, f, NewEligibilityChecker(testPool, eligibilityModel, resolver))
		require.ErrorIs(t, err, ErrPolicyUnavailable)
		require.False(t, ok)
		require.Empty(t, reason)
	}
	f.policy.DefinitionRef.DefinitionHash = "different-published-rule"
	ok, reason, err := checkEligibility(t, testPool, f, NewEligibilityChecker(testPool, eligibilityModel, f.resolver))
	require.ErrorIs(t, err, ErrPolicyUnavailable)
	require.False(t, ok)
	require.Empty(t, reason)
	boom := errors.New("policy publication read unavailable")
	_, _, err = checkEligibility(t, testPool, f, NewEligibilityChecker(testPool, eligibilityModel, func(context.Context, pgx.Tx, int64) (*AcceptancePolicy, error) { return nil, boom }))
	require.ErrorIs(t, err, boom)
}

// A single-connection pool and an uncommitted revocation distinguish exact Tx
// reads from hidden pool reads, which would either deadlock or miss revocation.
func TestEligibilityUsesExactGuardTransaction(t *testing.T) {
	f := newEligibilityFixture(t)
	cfg := testPool.Config()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()
	checker := NewEligibilityChecker(pool, eligibilityModel, f.resolver)
	ok, _, err := checkEligibility(t, pool, f, checker)
	require.NoError(t, err)
	require.True(t, ok)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, repository.WithIAMGuard(ctx, pool, func(ctx context.Context) error {
		tx := repository.IAMDB(ctx, pool).(pgx.Tx)
		_, err := tx.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, f.P)
		if err != nil {
			return err
		}
		ok, reason, err := checker(ctx, tx, f.TicketID, f.P)
		require.NoError(t, err)
		require.False(t, ok)
		require.Equal(t, "recipient_disabled", reason)
		return nil
	}))
	bare, err := pool.Begin(ctx)
	require.NoError(t, err)
	_, _, err = checker(ctx, bare, f.TicketID, f.P)
	require.ErrorIs(t, err, errcode.ErrNoPermission)
	require.NoError(t, bare.Rollback(ctx))
}

// A configured policy must match the dispatch-time publication and deadline;
// drift is retryable configuration failure, never a permanent suppression.
func TestEligibilityPolicyMismatchPreservesPendingIntent(t *testing.T) {
	for _, mismatch := range []string{"rule", "version", "deadline", "definition_and_state", "rule_and_state", "unknown_state"} {
		t.Run(mismatch, func(t *testing.T) {
			f := newEligibilityFixture(t)
			original := f.policy
			svc := newService(NewEligibilityChecker(testPool, eligibilityModel, f.resolver))
			_, err := svc.GenerateDue(context.Background(), tGen, 10)
			require.NoError(t, err)
			switch mismatch {
			case "rule":
				f.policy.RuleID = "another-policy"
			case "version":
				f.policy.RuleVersion++
			case "deadline":
				f.policy.AcceptWithin = time.Hour
			case "definition_and_state":
				f.policy.DefinitionRef.DefinitionVersion++
				f.policy.AwaitingState = "in_progress"
			case "rule_and_state":
				f.policy.RuleVersion++
				f.policy.AwaitingState = "in_progress"
			case "unknown_state":
				f.policy.AwaitingState = "unpublished_state"
			}
			report, err := svc.DeliverDue(context.Background(), tGen, 10)
			require.NoError(t, err)
			require.Len(t, report.Failed, 1)
			require.ErrorIs(t, report.Failed[0].Err, ErrPolicyUnavailable)
			require.Empty(t, report.Delivered)
			require.Empty(t, report.Suppressed)
			require.Zero(t, inboxCount(t, f.P))
			require.Equal(t, StatusPending, intentsOf(t, f.fixture)[0].Status)
			f.policy = original
			report, err = svc.DeliverDue(context.Background(), tGen, 10)
			require.NoError(t, err)
			require.Len(t, report.Delivered, 1)
			require.EqualValues(t, 1, inboxCount(t, f.P))
		})
	}
}

type eligibilityGuardTrace struct{ pid chan int32 }

func (trace *eligibilityGuardTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if data.SQL == `SELECT pg_advisory_lock($1)` {
		select {
		case trace.pid <- int32(conn.PgConn().PID()):
		default:
		}
	}
	return ctx
}

func (*eligibilityGuardTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

// A real repository disable on a second pool must wait for an admitted
// delivery; a disable committed first must suppress an already pending intent.
func TestEligibilityRealDisableOrdersWithDelivery(t *testing.T) {
	t.Run("delivery first blocks real disable", func(t *testing.T) {
		f := newEligibilityFixture(t)
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		entered := make(chan int32, 1)
		release := make(chan struct{})
		unblock := sync.OnceFunc(func() { close(release) })
		defer unblock()
		actual := NewEligibilityChecker(testPool, eligibilityModel, f.resolver)
		svc := newService(func(ctx context.Context, tx pgx.Tx, ticketID, recipient int64) (bool, string, error) {
			ok, reason, err := actual(ctx, tx, ticketID, recipient)
			if err != nil || !ok {
				return ok, reason, err
			}
			var pid int32
			if err := tx.QueryRow(ctx, `SELECT pg_backend_pid()`).Scan(&pid); err != nil {
				return false, "", err
			}
			entered <- pid
			select {
			case <-release:
				return true, "", nil
			case <-ctx.Done():
				return false, "", ctx.Err()
			}
		})
		_, err := svc.GenerateDue(ctx, tGen, 10)
		require.NoError(t, err)
		type deliveryResult struct {
			report *DeliveryReport
			err    error
		}
		done := make(chan deliveryResult, 1)
		go func() { r, err := svc.DeliverDue(ctx, tGen, 10); done <- deliveryResult{r, err} }()
		var admittedPID int32
		select {
		case admittedPID = <-entered:
		case <-ctx.Done():
			t.Fatal("delivery never reached current eligibility")
		}
		trace := &eligibilityGuardTrace{pid: make(chan int32, 1)}
		cfg := testPool.Config()
		cfg.ConnConfig.Tracer = trace
		writerPool, err := pgxpool.NewWithConfig(ctx, cfg)
		require.NoError(t, err)
		defer func() { unblock(); writerPool.Close() }()
		writerDone := make(chan error, 1)
		go func() { writerDone <- repository.NewUserRepo(writerPool).UpdateStatus(ctx, f.P, 0) }()
		var writerPID int32
		select {
		case writerPID = <-trace.pid:
		case <-ctx.Done():
			t.Fatal("real account writer never attempted IAM barrier")
		}
		require.Eventually(t, func() bool {
			var blocked bool
			err := testPool.QueryRow(ctx, `SELECT $1::int=ANY(pg_blocking_pids($2))`, admittedPID, writerPID).Scan(&blocked)
			return err == nil && blocked
		}, 3*time.Second, 10*time.Millisecond)
		unblock()
		select {
		case result := <-done:
			require.NoError(t, result.err)
			require.Len(t, result.report.Delivered, 1)
		case <-ctx.Done():
			t.Fatal("admitted delivery did not finish")
		}
		select {
		case err := <-writerDone:
			require.NoError(t, err)
		case <-ctx.Done():
			t.Fatal("real account disable did not finish")
		}
		require.EqualValues(t, 1, inboxCount(t, f.P))
	})
	t.Run("real disable commits before delivery", func(t *testing.T) {
		f := newEligibilityFixture(t)
		svc := newService(NewEligibilityChecker(testPool, eligibilityModel, f.resolver))
		_, err := svc.GenerateDue(context.Background(), tGen, 10)
		require.NoError(t, err)
		require.NoError(t, repository.NewUserRepo(testPool).UpdateStatus(context.Background(), f.P, 0))
		report, err := svc.DeliverDue(context.Background(), tGen, 10)
		require.NoError(t, err)
		require.Empty(t, report.Delivered)
		require.Len(t, report.Suppressed, 1)
		require.Equal(t, "recipient_disabled", *report.Suppressed[0].SuppressReason)
		require.Zero(t, inboxCount(t, f.P))
	})
}

//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// Both commands must pass the initial empty receipt read before either can
// write. Removing the replay read after the ticket lock makes the second
// identical command fail with a revision conflict instead of the first result.
func TestD02ConcurrentReplayAfterTicketLock(t *testing.T) {
	for _, http := range []bool{false, true} {
		for _, kind := range []string{"same_request", "changed_payload", "different_key"} {
			t.Run(strconv.FormatBool(http)+"/"+kind, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
				defer cancel()
				svc, actor, ticket, req, effects := newD02ReplayFixture(t, ctx)
				if http {
					ctx = d02ReplayHTTPContext(t, ctx, actor)
				}
				reg := domainevent.NewHookRegistry(nil)
				var events atomic.Int32
				reg.Subscribe(domainevent.BizTicket, "d02.replay", domainevent.HookFunc(func(_ context.Context, _ domainevent.DomainEvent) { events.Add(1) }))
				svc.SetEventRegistry(reg)

				prior, err := testPool.Begin(ctx)
				require.NoError(t, err)
				defer prior.Rollback(context.Background())
				var blocker int
				require.NoError(t, prior.QueryRow(ctx, `SELECT pg_backend_pid() FROM tickets WHERE id=$1 FOR UPDATE`, ticket.ID).Scan(&blocker))
				type result struct {
					ticket *model.Ticket
					err    error
				}
				done := make(chan result, 2)
				other := *req
				switch kind {
				case "changed_payload":
					other.Comment = "different submission"
				case "different_key":
					other.IdempotencyKey += "-other"
				}
				for _, command := range []*model.TransitionTicketRequest{req, &other} {
					go func() {
						got, err := svc.Transition(ctx, command, actor)
						done <- result{got, err}
					}()
				}
				require.Eventually(t, func() bool {
					var blocked int
					err := testPool.QueryRow(ctx, `WITH RECURSIVE waiting(pid) AS (
						SELECT $1::integer UNION SELECT a.pid FROM pg_stat_activity a
						JOIN waiting w ON w.pid=ANY(pg_blocking_pids(a.pid)))
						SELECT count(*) FROM pg_stat_activity WHERE pid IN (SELECT pid FROM waiting WHERE pid<>$1)
						AND query LIKE 'SELECT%FROM tickets WHERE id%FOR UPDATE%'`, blocker).Scan(&blocked)
					return err == nil && blocked == 2
				}, 5*time.Second, 5*time.Millisecond, "both commands must miss the receipt and wait for the ticket lock")
				require.NoError(t, prior.Commit(ctx))
				first, second := <-done, <-done
				if first.err != nil {
					first, second = second, first
				}
				require.NoError(t, first.err)
				require.Equal(t, StatusAssigned, first.ticket.Status)
				require.Equal(t, int64(2), first.ticket.Revision)
				switch kind {
				case "same_request":
					require.NoError(t, second.err, "same K and payload must replay the winning commit")
					require.Equal(t, first.ticket, second.ticket)
				case "changed_payload":
					require.ErrorIs(t, second.err, errcode.ErrConflict)
				case "different_key":
					require.ErrorIs(t, second.err, errcode.ErrConcurrentModification)
				}
				require.NoError(t, reg.Drain(ctx))
				require.Equal(t, int32(1), events.Load(), "replay must not emit a second domain event")
				require.Equal(t, int32(1), effects.Load(), "replay must not run post_actions again")
				var revision int64
				var status string
				var data json.RawMessage
				var attachments, changes, comments, receipts int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT revision,status,custom_data,
					(SELECT count(*) FROM ticket_attachments WHERE ticket_id=t.id),
					(SELECT count(*) FROM ticket_events WHERE ticket_id=t.id AND action='status_changed'),
					(SELECT count(*) FROM ticket_comments WHERE ticket_id=t.id),
					(SELECT count(*) FROM ticket_evidence_transition_receipts WHERE ticket_id=t.id)
					FROM tickets t WHERE id=$1`, ticket.ID).Scan(&revision, &status, &data, &attachments, &changes, &comments, &receipts))
				require.Equal(t, int64(2), revision)
				require.Equal(t, StatusAssigned, status)
				require.JSONEq(t, string(req.CustomDataPatch), string(data))
				require.Equal(t, []int{1, 1, 1, 1}, []int{attachments, changes, comments, receipts})
			})
		}
	}
}

func TestD02ReplayRevalidatesCurrentAuthorization(t *testing.T) {
	for _, change := range []string{"advanced", "epoch", "status", "role", "policy", "ticket_hidden", "ticket_read_only", "active_risk_link"} {
		t.Run(change, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			svc, actor, ticket, req, effects := newD02ReplayFixture(t, ctx)
			ctx = d02ReplayHTTPContext(t, ctx, actor)
			original, err := svc.Transition(ctx, req, actor)
			require.NoError(t, err)
			want := errcode.ErrNoPermission
			switch change {
			case "advanced":
				_, err = testPool.Exec(ctx, `UPDATE tickets SET status='in_progress' WHERE id=$1`, ticket.ID)
			case "epoch":
				_, err = testPool.Exec(ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, actor)
			case "status":
				_, err = testPool.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, actor)
			case "role":
				_, err = testPool.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1 AND role_id=(SELECT id FROM roles WHERE code=$2)`, actor, "d02_http_"+strconv.FormatInt(actor, 10))
			case "policy":
				_, err = testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE ptype='p' AND v0=$1 AND v1='/api/v1/tickets/transition' AND v2='POST'`, "role::d02_http_"+strconv.FormatInt(actor, 10))
			case "ticket_hidden", "ticket_read_only":
				var other int64
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password,status) VALUES($1,'test-hash',1) RETURNING id`, "d02_other_"+uniqueSuffix()).Scan(&other))
				_, err = testPool.Exec(ctx, `UPDATE tickets SET created_by=$2 WHERE id=$1`, ticket.ID, other)
				require.NoError(t, err)
				if change == "ticket_read_only" {
					_, err = testPool.Exec(ctx, `INSERT INTO ticket_cc(ticket_id,user_id,created_by) VALUES($1,$2,$3)`, ticket.ID, actor, other)
				} else {
					want = errcode.ErrTicketNotFound
				}
			case "active_risk_link":
				tx, e := testPool.Begin(ctx)
				require.NoError(t, e)
				defer tx.Rollback(context.Background())
				seedD02ReplayRiskBinding(t, ctx, tx, ticket, actor)
				err = tx.Commit(ctx)
			}
			require.NoError(t, err)
			replayed, err := svc.Transition(ctx, req, actor)
			if change == "advanced" {
				require.NoError(t, err)
				require.Equal(t, original, replayed, "return the original receipt, not the advanced ticket")
			} else {
				require.ErrorIs(t, err, want)
				require.Nil(t, replayed)
			}
			require.Equal(t, int32(1), effects.Load())
			var receipts, comments int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM ticket_evidence_transition_receipts WHERE ticket_id=$1),
				(SELECT count(*) FROM ticket_comments WHERE ticket_id=$1)`, ticket.ID).Scan(&receipts, &comments))
			require.Equal(t, []int{1, 1}, []int{receipts, comments})
		})
	}
}

// An unrelated revision change must still conflict. A risk binding appearing
// during the wait must reject the ordinary command even if a matching receipt
// becomes visible in the same preceding commit.
func TestD02WaitingCommandPreservesCASAndRiskGuard(t *testing.T) {
	for _, change := range []string{"revision", "risk_link", "risk_link_and_receipt"} {
		t.Run(change, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			svc, actor, ticket, req, effects := newD02ReplayFixture(t, ctx)
			ctx = d02ReplayHTTPContext(t, ctx, actor)
			prior, err := testPool.Begin(ctx)
			require.NoError(t, err)
			defer prior.Rollback(context.Background())
			var blocker int
			require.NoError(t, prior.QueryRow(ctx, `SELECT pg_backend_pid() FROM tickets WHERE id=$1 FOR UPDATE`, ticket.ID).Scan(&blocker))
			done := make(chan error, 1)
			go func() {
				_, err := svc.Transition(ctx, req, actor)
				done <- err
			}()
			waitSnapshotBlocker(t, ctx, blocker)
			want := errcode.ErrNoPermission
			written := 0
			if change == "revision" {
				_, err = prior.Exec(ctx, `UPDATE tickets SET priority=1 WHERE id=$1`, ticket.ID)
				require.NoError(t, err)
				want = errcode.ErrConcurrentModification
			} else {
				if change == "risk_link_and_receipt" {
					snapshot, _, err := svc.ticketRepo.BoundTicketDefinitionTx(ctx, prior, ticket.TypeCode, ticket.DefinitionVersion)
					require.NoError(t, err)
					cmd, err := parseEvidenceTransitionRequest(req)
					require.NoError(t, err)
					_, err = svc.applyEvidenceTransitionTx(ctx, prior, cmd, ticket, snapshot, actor)
					require.NoError(t, err)
					written = 1
				}
				seedD02ReplayRiskBinding(t, ctx, prior, ticket, actor)
			}
			require.NoError(t, prior.Commit(ctx))
			require.ErrorIs(t, <-done, want)
			require.Zero(t, effects.Load(), "blocked command cannot release post_actions")
			var receipts, attachments, comments int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT
				(SELECT count(*) FROM ticket_evidence_transition_receipts WHERE ticket_id=$1),
				(SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1),
				(SELECT count(*) FROM ticket_comments WHERE ticket_id=$1)`, ticket.ID).Scan(&receipts, &attachments, &comments))
			require.Equal(t, []int{written, written, written}, []int{receipts, attachments, comments})
		})
	}
}

func seedD02ReplayRiskBinding(t *testing.T, ctx context.Context, tx pgx.Tx, ticket *model.Ticket, actor int64) {
	t.Helper()
	key := "d02_risk_" + uniqueSuffix()
	var group, family, risk int64
	require.NoError(t, tx.QueryRow(ctx, `INSERT INTO risk_dispatch_groups
		(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,
		 approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id)
		VALUES($1,$2,$3,$3,$4,$5,'synthetic',$6,$7,$8,$2) RETURNING id`,
		key, actor, ticket.OrgID, ticket.TypeCode, *ticket.DefinitionVersion, strings.Repeat("d", 64), ticket.ID, ticket.HumanKey).Scan(&group))
	require.NoError(t, tx.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope,canonical_key_hash)
		VALUES('synthetic',$1) RETURNING id`, key).Scan(&family))
	require.NoError(t, tx.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by)
		VALUES($1,$2,'remediating',$3,$4,$1,$4) RETURNING id`, key, family, ticket.OrgID, actor).Scan(&risk))
	_, err := tx.Exec(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3)`, risk, group, actor)
	require.NoError(t, err)
}

type d02ReplayEffect struct{ calls atomic.Int32 }

func (*d02ReplayEffect) Name() string                { return "d02_replay_effect" }
func (*d02ReplayEffect) Delivery() DeliverySemantics { return DeliveryAtMostOnce }
func (a *d02ReplayEffect) Execute(context.Context, ActionContext) error {
	a.calls.Add(1)
	return nil
}

func newD02ReplayFixture(t *testing.T, ctx context.Context) (*Service, int64, *model.Ticket, *model.TransitionTicketRequest, *atomic.Int32) {
	t.Helper()
	svc, actor, org := setupTicketAdmin(t)
	effect := &d02ReplayEffect{}
	actions := NewActionRegistry()
	require.NoError(t, actions.Register(effect))
	svc.SetActionRegistry(actions)
	code := "d02_replay_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{
		{FieldKey: "plan", FieldLabel: "Plan", FieldType: "input"},
		{FieldKey: "files", FieldLabel: "Files", FieldType: "file"},
	}})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{
		"open>assigned":{"collect_fields":[{"key":"plan","required":true},{"key":"files","required":true}],
		"validator":"custom_data['plan'] == 'fixed'","post_actions":["d02_replay_effect"]}}`)})
	require.NoError(t, err)
	def := publishD01(t, svc, code, actor)
	ticket, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, OrgID: org()}, actor)
	require.NoError(t, err)
	file := "synthetic/d02/replay-" + uniqueSuffix()
	seedPendingObject(t, actor, file, "image/png")
	patch, err := json.Marshal(map[string]any{"plan": "fixed", "files": []string{file}})
	require.NoError(t, err)
	return svc, actor, ticket, &model.TransitionTicketRequest{
		ID: ticket.ID, To: StatusAssigned, Comment: "synthetic submission", IdempotencyKey: code,
		ExpectedRevision: rawJSON(`"1"`), DefinitionVersion: rawJSON(strconv.Quote(strconv.FormatInt(def.DefinitionVersion, 10))),
		CustomDataPatch: patch,
	}, &effect.calls
}

// Retain a successfully admitted middleware identity while the service reads
// current credentials and L1/L2/L3 facts inside its own C04 transaction.
func d02ReplayHTTPContext(t *testing.T, ctx context.Context, actor int64) context.Context {
	t.Helper()
	const path = "/api/v1/tickets/transition"
	role := "d02_http_" + strconv.FormatInt(actor, 10)
	var roleID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority) VALUES($1,$1,50) RETURNING id`, role).Scan(&roleID))
	_, err := testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, actor, roleID)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+role, path)
	require.NoError(t, err)
	m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	_, err = enforcer.AddPolicy("role::"+role, path, "POST")
	require.NoError(t, err)
	var epoch int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("userID", actor); c.Set("session_epoch", epoch); c.Next() }, middleware.CasbinAuth(enforcer, stubRoleFetcher{actor: {role}}, nil))
	var admitted context.Context
	router.POST(path, func(c *gin.Context) { admitted = c.Request.Context() })
	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest("POST", path, nil).WithContext(ctx))
	require.Equal(t, 200, response.Code)
	require.NotNil(t, admitted)
	return admitted
}

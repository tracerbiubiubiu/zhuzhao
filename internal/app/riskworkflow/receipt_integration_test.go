//go:build integration

package riskworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type receiptFixture struct {
	ctx                                                   context.Context
	tickets                                               *ticket.Service
	actor, other, orgA, orgB, ticket, riskA, riskB, linkB int64
	role, operation                                       string
	want                                                  riskremediation.Result
	enforcer                                              *casbin.SyncedEnforcer
}

const receiptPathPrefix = "/api/v1/risk-items/remediation-operations/"
const receiptModel = `[request_definition]
r = sub, obj, act
[policy_definition]
p = sub, obj, act
[policy_effect]
e = some(where (p.eft == allow))
[matchers]
m = r.sub == p.sub && keyMatch2(r.obj,p.obj) && r.act == p.act`

func newReceiptFixture(t *testing.T) *receiptFixture {
	return newPreparedReceiptFixture(t, nil, nil)
}
func newPreparedReceiptFixture(t *testing.T, prepareType func(*receiptFixture, string), prepareTicket func(*receiptFixture, *model.Ticket)) *receiptFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	t.Cleanup(cancel)
	f := &receiptFixture{ctx: ctx, tickets: workflowTicketService()}
	suffix := fmt.Sprint(time.Now().UnixNano())
	f.role = "wf_" + suffix
	f.operation = "synthetic-original-" + suffix
	for _, id := range []*int64{&f.actor, &f.other} {
		require.NoError(t, workflowPool.QueryRow(ctx, `INSERT INTO users(username,password,status) VALUES($1,'synthetic-non-login',1) RETURNING id`, f.role+fmt.Sprint(id == &f.actor)).Scan(id))
	}
	for i, id := range []*int64{&f.orgA, &f.orgB} {
		code := fmt.Sprintf("wf%s%d", suffix, i)
		require.NoError(t, workflowPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path) VALUES($1::text,$1::text,$1::text::ltree) RETURNING id`, code).Scan(id))
		_, err := workflowPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, f.actor, *id)
		require.NoError(t, err)
	}
	var role int64
	require.NoError(t, workflowPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority) VALUES($1::text,$1::text,50) RETURNING id`, f.role).Scan(&role))
	_, err := workflowPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, f.actor, role)
	require.NoError(t, err)
	code := "wf" + suffix
	_, err = f.tickets.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "Synthetic remediation receipt"})
	require.NoError(t, err)
	if prepareType != nil {
		prepareType(f, code)
	}
	draft, err := repository.NewTicketRepo(workflowPool).GetTicketType(ctx, code)
	require.NoError(t, err)
	def, _, err := f.tickets.PublishTicketType(ctx, draft, f.actor)
	require.NoError(t, err)
	// A real owning Create binds the published rule; fixture SQL never changes tickets.
	tk, err := f.tickets.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "Synthetic original receipt ticket", OrgID: f.orgA}, f.actor)
	require.NoError(t, err)
	f.ticket = tk.ID
	if prepareTicket != nil {
		prepareTicket(f, tk)
	}
	var group int64
	require.NoError(t, workflowPool.QueryRow(ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id) VALUES($1,$2,$3,$3,$4,$5,'synthetic',repeat('b',64),$6,$7,$2) RETURNING id`, f.role, f.other, f.orgA, code, def.DefinitionVersion, f.ticket, tk.HumanKey).Scan(&group))
	for i, id := range []*int64{&f.riskA, &f.riskB} {
		var family, link int64
		require.NoError(t, workflowPool.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('synthetic',$1) RETURNING id`, fmt.Sprintf("%s-%d", f.role, i)).Scan(&family))
		org := f.orgA
		if i == 1 {
			org = f.orgB
		}
		require.NoError(t, workflowPool.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by) VALUES($1,$2,'remediating',$3,$4,$1,$4) RETURNING id`, fmt.Sprintf("%s-%d", f.role, i), family, org, f.other).Scan(id))
		require.NoError(t, workflowPool.QueryRow(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3) RETURNING id`, *id, group, f.other).Scan(&link))
		if i == 1 {
			f.linkB = link
		}
		f.want.Items = append(f.want.Items, riskremediation.ItemResult{LinkID: link, RiskID: *id, LinkRevision: 1, RiskRevision: 1, ProposalVersion: 1, Result: "pending_confirmation"})
	}
	f.want.TicketID = f.ticket
	f.want.TicketRevision = tk.Revision
	// This fixture represents an already committed historic operation, not proof
	// that finish/review can currently cross the blocked atomic child boundary.
	body, err := json.Marshal(f.want)
	require.NoError(t, err)
	_, err = workflowPool.Exec(ctx, `INSERT INTO risk_operations(actor_id,operation_id,request_hash,command,result_status) VALUES($1,$2,decode(repeat('ab',32),'hex'),'remediation_finish','completed')`, f.actor, f.operation)
	require.NoError(t, err)
	_, err = workflowPool.Exec(ctx, `INSERT INTO risk_remediation_receipts(actor_id,operation_id,ticket_id,result) VALUES($1,$2,$3,$4)`, f.actor, f.operation, f.ticket, body)
	require.NoError(t, err)
	model, err := casbinmodel.NewModelFromString(receiptModel)
	require.NoError(t, err)
	f.enforcer, err = casbin.NewSyncedEnforcer(model)
	require.NoError(t, err)
	for _, path := range []string{receiptPathPrefix + ":id", "/api/v1/tickets/:id"} {
		_, err = workflowPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'GET')`, "role::"+f.role, path)
		require.NoError(t, err)
		_, err = f.enforcer.AddPolicy("role::"+f.role, path, "GET")
		require.NoError(t, err)
	}
	return f
}

// The HTTP seam is the real purpose-minting middleware. The mutation runs only
// after admission, so stale middleware admission cannot authorize service reads.
func (f *receiptFixture) read(t *testing.T, op string, before func()) (riskremediation.Result, error) {
	return f.admitted(t, op, before, func(ctx context.Context) (riskremediation.Result, error) {
		return NewReceiptService(workflowPool, f.tickets).Receipt(ctx, f.actor, op)
	})
}
func (f *receiptFixture) admitted(t *testing.T, op string, before func(), run func(context.Context) (riskremediation.Result, error)) (riskremediation.Result, error) {
	t.Helper()
	var epoch int64
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT session_epoch FROM users WHERE id=$1`, f.actor).Scan(&epoch))
	var got riskremediation.Result
	var readErr error
	called := false
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", f.actor); c.Set("session_epoch", epoch); c.Next() }, middleware.CasbinAuth(f.enforcer, databaseRoles{workflowPool}, nil))
	r.GET(receiptPathPrefix+":id", func(c *gin.Context) {
		called = true
		if before != nil {
			before()
		}
		got, readErr = run(c.Request.Context())
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", receiptPathPrefix+op, nil).WithContext(f.ctx))
	require.True(t, called, "real middleware must admit the fixture before transaction recheck")
	return got, readErr
}

func TestReceiptOriginalMembersAndCurrentAuthority(t *testing.T) {
	gin.SetMode(gin.TestMode)
	t.Run("same version success and repeat are read only", func(t *testing.T) {
		f := newReceiptFixture(t)
		before := f.snapshot(t)
		for range 2 {
			got, err := f.read(t, f.operation, nil)
			require.NoError(t, err)
			require.Equal(t, f.want, got)
		}
		require.Equal(t, before, f.snapshot(t))
	})
	t.Run("detached original risk still authorizes", func(t *testing.T) {
		f := newReceiptFixture(t)
		_, err := workflowPool.Exec(f.ctx, `UPDATE risk_ticket_links SET ended_at=clock_timestamp(),end_reason='synthetic transfer' WHERE id=$1`, f.linkB)
		require.NoError(t, err)
		got, err := f.read(t, f.operation, nil)
		require.NoError(t, err)
		require.Equal(t, f.want, got)
		before := f.snapshot(t)
		got, err = f.read(t, f.operation, func() {
			_, e := workflowPool.Exec(f.ctx, `DELETE FROM user_orgs WHERE user_id=$1 AND org_id=$2`, f.actor, f.orgB)
			require.NoError(t, e)
		})
		require.ErrorIs(t, err, errcode.ErrNotFound)
		require.Empty(t, got)
		require.Equal(t, before, f.snapshot(t))
	})
	for _, kind := range []string{"risk route policy", "ticket route policy", "role", "epoch", "disabled"} {
		t.Run(kind, func(t *testing.T) {
			f := newReceiptFixture(t)
			_, err := f.read(t, f.operation, nil)
			require.NoError(t, err)
			before := f.snapshot(t)
			got, err := f.read(t, f.operation, func() {
				var e error
				switch kind {
				case "risk route policy":
					_, e = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2`, "role::"+f.role, receiptPathPrefix+":id")
				case "ticket route policy":
					_, e = workflowPool.Exec(f.ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets/:id'`, "role::"+f.role)
				case "role":
					_, e = workflowPool.Exec(f.ctx, `DELETE FROM user_roles WHERE user_id=$1`, f.actor)
				case "epoch":
					_, e = workflowPool.Exec(f.ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, f.actor)
				case "disabled":
					_, e = workflowPool.Exec(f.ctx, `UPDATE users SET status=0 WHERE id=$1`, f.actor)
				}
				require.NoError(t, e)
			})
			require.Error(t, err)
			require.Empty(t, got)
			require.Equal(t, before, f.snapshot(t))
		})
	}
	t.Run("other actor original number and absent are same", func(t *testing.T) {
		f := newReceiptFixture(t)
		_, err := workflowPool.Exec(f.ctx, `INSERT INTO user_roles(user_id,role_id) SELECT $1,id FROM roles WHERE code=$2`, f.other, f.role)
		require.NoError(t, err)
		f.actor = f.other
		first, e1 := f.read(t, f.operation, nil)
		second, e2 := f.read(t, "absent-synthetic-operation", nil)
		require.ErrorIs(t, e1, errcode.ErrNotFound)
		require.ErrorIs(t, e2, errcode.ErrNotFound)
		require.Empty(t, first)
		require.Empty(t, second)
	})
	t.Run("missing middleware purpose stays closed", func(t *testing.T) {
		f := newReceiptFixture(t)
		got, err := NewReceiptService(workflowPool, f.tickets).Receipt(f.ctx, f.actor, f.operation)
		require.ErrorIs(t, err, errcode.ErrNoPermission)
		require.Empty(t, got)
	})

}
func (f *receiptFixture) snapshot(t *testing.T) string {
	t.Helper()
	var got string
	require.NoError(t, workflowPool.QueryRow(f.ctx, `SELECT jsonb_build_object('ticket',(SELECT to_jsonb(t) FROM tickets t WHERE id=$1),'events',(SELECT jsonb_agg(to_jsonb(e) ORDER BY id) FROM ticket_events e WHERE ticket_id=$1),'risk',(SELECT jsonb_agg(to_jsonb(r) ORDER BY id) FROM risk_items r WHERE id=ANY($2)),'links',(SELECT jsonb_agg(to_jsonb(l) ORDER BY id) FROM risk_ticket_links l WHERE risk_id=ANY($2)),'receipts',(SELECT jsonb_agg(to_jsonb(r)) FROM risk_remediation_receipts r WHERE actor_id=$3 AND operation_id=$4),'intents',(SELECT jsonb_agg(to_jsonb(i)) FROM risk_remediation_intents i WHERE actor_id=$3 AND operation_id=$4))::text`, f.ticket, []int64{f.riskA, f.riskB}, f.actor, f.operation).Scan(&got))
	return got
}

func TestReceiptTicketOnlyAuthorizationCallback(t *testing.T) {
	f := newReceiptFixture(t)
	_, err := f.admitted(t, f.operation, nil, func(ctx context.Context) (riskremediation.Result, error) {
		err := repository.WithIAMCommandGuard(ctx, workflowPool, func(ctx context.Context, tx pgx.Tx) error {
			return f.tickets.WithRiskWorkflowAuthorizationTx(ctx, tx, f.actor, []ticket.RiskWorkflowRoute{{Path: receiptPathPrefix + f.operation, Method: "GET"}}, func(ctx context.Context) error {
				return NewReceiptService(workflowPool, f.tickets).receiptCallbacks().Authorize(ctx, tx, riskremediation.Access{Action: "receipt", ActorID: f.actor, TicketID: f.ticket})
			})
		})
		return riskremediation.Result{}, err
	})
	require.NoError(t, err, "kernel authorizes the original ticket before enumerating original risks")
}

// Positive control exercises the SAME kernel and same schema without a ticket.
// This is historical unlinked acceptance receipt recovery, not finish evidence.
func TestReceiptUnlinkedOriginalPositiveControl(t *testing.T) {
	f := newReceiptFixture(t)
	f.want.TicketID = 0
	f.want.TicketRevision = 0
	for i := range f.want.Items {
		f.want.Items[i].LinkID = 0
	}
	body, err := json.Marshal(f.want)
	require.NoError(t, err)
	_, err = workflowPool.Exec(f.ctx, `UPDATE risk_remediation_receipts SET ticket_id=NULL,result=$3 WHERE actor_id=$1 AND operation_id=$2`, f.actor, f.operation, body)
	require.NoError(t, err)
	before := f.snapshot(t)
	for range 2 {
		got, err := f.read(t, f.operation, nil)
		require.NoError(t, err)
		require.Equal(t, f.want, got)
	}
	require.Equal(t, before, f.snapshot(t))
	got, err := f.read(t, f.operation, func() {
		_, e := workflowPool.Exec(f.ctx, `DELETE FROM user_orgs WHERE user_id=$1 AND org_id=$2`, f.actor, f.orgB)
		require.NoError(t, e)
	})
	require.ErrorIs(t, err, errcode.ErrNotFound)
	require.Empty(t, got)
	require.Equal(t, before, f.snapshot(t))
}

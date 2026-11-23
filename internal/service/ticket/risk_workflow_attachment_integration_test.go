//go:build integration

package ticket

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	orgdeleg "github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"log/slog"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

type riskAttachmentFixture struct {
	svc                                         *Service
	storage                                     *storageservice.Service
	actor, creator, viewer, ticket, group, risk int64
	role                                        string
}

func newRiskAttachmentFixture(t *testing.T) *riskAttachmentFixture {
	t.Helper()
	svc, creator, actor, viewer, roles := setupTicket2a(t)
	roles[creator] = []string{"admin"}
	ctx := context.Background()
	code := "upload_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{
		Transitions:    rawJSON(`{"open":["assigned"],"assigned":["in_progress"],"in_progress":["pending_verify"],"pending_verify":["pending_verify","closed","in_progress"],"closed":["open"]}`),
		TransitionMeta: rawJSON(`{"in_progress>pending_verify":{"roles":["assignee"]}}`),
	})
	require.NoError(t, err)
	def := publishD01(t, svc, code, creator)
	var org int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM organizations WHERE code='root'`).Scan(&org))
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: code, OrgID: org}, creator)
	require.NoError(t, err)
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &actor}, creator))
	_, err = testPool.Exec(ctx, `UPDATE tickets SET status='in_progress' WHERE id=$1`, tk.ID)
	require.NoError(t, err)
	f := &riskAttachmentFixture{svc: svc, actor: actor, creator: creator, viewer: viewer, ticket: tk.ID, role: code}
	var family, role int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id) VALUES($1,$2,$3,$3,$1,$4,'synthetic',$5,$6,$7,$2) RETURNING id`, code, creator, org, def.DefinitionVersion, strings.Repeat("e", 64), tk.ID, tk.HumanKey).Scan(&f.group))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('synthetic',$1) RETURNING id`, code).Scan(&family))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by) VALUES($1,$2,'remediating',$3,$4,$1,$4) RETURNING id`, code, family, org, creator).Scan(&f.risk))
	_, err = testPool.Exec(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3)`, f.risk, f.group, creator)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, actor, org)
	require.NoError(t, err)
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority) VALUES($1,$1,50) RETURNING id`, code).Scan(&role))
	_, err = testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2),($3,$2)`, actor, role, viewer)
	require.NoError(t, err)
	for _, route := range []struct{ path, method string }{
		{"/api/v1/risk-items/remediation-finish", "POST"}, {"/api/v1/tickets/transition", "POST"}, {"/api/v1/tickets/" + strconv.FormatInt(tk.ID, 10), "GET"},
	} {
		_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3)`, "role::"+code, route.path, route.method)
		require.NoError(t, err)
	}
	cfg := &pkgstorage.StorageConfig{Enabled: true, Bucket: code, PresignUploadTTL: "15m"}
	cfg.ApplyDefaults()
	f.storage = storageservice.NewService(repository.NewStorageRepo(testPool), &b53StubClient{info: &pkgstorage.ObjectInfo{Size: 10, ETag: "synthetic", ContentType: "image/png"}}, pkgstorage.NewConfigStore(cfg), slog.Default())
	f.storage.UseTicketAttachmentCommand(NewRiskWorkflowAttachmentCommand(svc, true))
	return f
}

func (f *riskAttachmentFixture) request(t *testing.T, actor int64, path string, before func(), run func(context.Context) error) error {
	t.Helper()
	var epoch int64
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
	m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
	require.NoError(t, err)
	e, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", actor); c.Set("session_epoch", epoch); c.Next() }, middleware.SelfService(), middleware.CasbinAuth(e, stubRoleFetcher{actor: {f.role}}, nil))
	var result error
	called := false
	r.POST(path, func(c *gin.Context) {
		called = true
		if before != nil {
			before()
		}
		result = run(c.Request.Context())
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("POST", path, nil))
	require.True(t, called)
	return result
}

func (f *riskAttachmentFixture) presign(t *testing.T, actor int64, purpose string) (string, error) {
	t.Helper()
	var key string
	err := f.request(t, actor, "/api/v1/storage/presign/upload", nil, func(ctx context.Context) error {
		out, err := f.storage.PresignUpload(ctx, &model.PresignUploadInput{Purpose: purpose, TicketID: &f.ticket, FileName: "synthetic.png", MIMEType: "image/png", SizeBytes: 10}, actor)
		if out != nil {
			key = out.ObjectKey
		}
		return err
	})
	return key, err
}

func TestRiskWorkflowAttachmentCurrentHandler(t *testing.T) {
	f := newRiskAttachmentFixture(t)
	require.Error(t, f.svc.AuthorizeTicketAttachment(context.Background(), f.actor, f.ticket), "ordinary update remains creator-only")
	key, err := f.presign(t, f.actor, "ticket_attachment")
	require.NoError(t, err, "current P must upload finish evidence without ordinary update")
	err = f.request(t, f.actor, "/api/v1/tickets/attachments/confirm", nil, func(ctx context.Context) error {
		_, err := f.storage.Confirm(ctx, &model.ConfirmFileInput{ObjectKey: key, TicketID: &f.ticket}, f.actor)
		return err
	})
	require.NoError(t, err)
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, f.ticket).Scan(&n))
	require.Equal(t, 1, n)
	err = f.request(t, f.actor, "/api/v1/tickets/attachments/delete", nil, func(ctx context.Context) error {
		return f.storage.DeleteTicketAttachmentAs(ctx, f.actor, f.ticket, key)
	})
	require.ErrorIs(t, err, errcode.ErrNoPermission, "upload eligibility must never grant delete")
}

func TestRiskWorkflowAttachmentRejectsCurrentIneligibility(t *testing.T) {
	for _, change := range []string{"finish_policy", "transition_policy", "read_policy", "role", "epoch", "disabled_user", "reassigned", "assigned", "pending_verify", "closed_group", "detached", "risk_hidden", "no_actionable", "pin", "disabled_flag", "viewer", "richtext"} {
		t.Run(change, func(t *testing.T) {
			f := newRiskAttachmentFixture(t)
			actor, purpose := f.actor, "ticket_attachment"
			if change == "viewer" {
				actor = f.viewer
			}
			if change == "richtext" {
				purpose = "ticket_richtext"
			}
			if change == "disabled_flag" {
				f.storage.UseTicketAttachmentCommand(NewRiskWorkflowAttachmentCommand(f.svc, false))
			}
			before := func() {
				require.NoError(t, repository.WithIAMGuard(context.Background(), testPool, func(ctx context.Context) error {
					q := repository.IAMDB(ctx, testPool)
					var err error
					switch change {
					case "finish_policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/risk-items/remediation-finish'`, "role::"+f.role)
					case "transition_policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/tickets/transition'`, "role::"+f.role)
					case "read_policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v2='GET'`, "role::"+f.role)
					case "role":
						_, err = q.Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1`, f.actor)
					case "epoch":
						_, err = q.Exec(ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, f.actor)
					case "disabled_user":
						_, err = q.Exec(ctx, `UPDATE users SET status=0 WHERE id=$1`, f.actor)
					case "reassigned":
						_, err = q.Exec(ctx, `UPDATE tickets SET assigned_to=$2 WHERE id=$1`, f.ticket, f.viewer)
					case "assigned", "pending_verify":
						_, err = q.Exec(ctx, `UPDATE tickets SET status=$2 WHERE id=$1`, f.ticket, change)
					case "closed_group":
						_, err = q.Exec(ctx, `UPDATE risk_dispatch_groups SET ended_at=NOW() WHERE id=$1`, f.group)
					case "detached":
						_, err = q.Exec(ctx, `UPDATE risk_ticket_links SET ended_at=NOW(),end_reason='ticket_closed' WHERE group_id=$1`, f.group)
					case "risk_hidden":
						_, err = q.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1`, f.actor)
					case "no_actionable":
						_, err = q.Exec(ctx, `UPDATE risk_ticket_links SET remediation_state='pending_verify' WHERE group_id=$1`, f.group)
					case "pin":
						_, err = q.Exec(ctx, `UPDATE tickets SET definition_version=NULL WHERE id=$1`, f.ticket)
					}
					return err
				}))
			}
			err := f.request(t, actor, "/api/v1/storage/presign/upload", before, func(ctx context.Context) error {
				_, err := f.storage.PresignUpload(ctx, &model.PresignUploadInput{Purpose: purpose, TicketID: &f.ticket, FileName: "synthetic.png", MIMEType: "image/png", SizeBytes: 10}, actor)
				return err
			})
			require.Error(t, err, change)
			var n int
			require.NoError(t, testPool.QueryRow(context.Background(), `SELECT count(*) FROM file_objects WHERE bucket=$1`, f.role).Scan(&n))
			require.Zero(t, n, "no reservation survives a rejected command")
		})
	}
}

// HEAD runs outside both preflight and persistent IAM guards. These changes can
// commit during HEAD, and the persistent confirm must then reject without binding.
type riskAttachmentHeadClient struct {
	b53StubClient
	head func()
}

func (c *riskAttachmentHeadClient) StatObject(ctx context.Context, key string) (*pkgstorage.ObjectInfo, error) {
	if c.head != nil {
		c.head()
	}
	return c.b53StubClient.StatObject(ctx, key)
}

func TestRiskWorkflowAttachmentConfirmRechecksAfterHEAD(t *testing.T) {
	for _, change := range []string{"epoch", "finish_policy", "reassigned", "closed_group"} {
		t.Run(change, func(t *testing.T) {
			f := newRiskAttachmentFixture(t)
			cfg := &pkgstorage.StorageConfig{Enabled: true, Bucket: f.role, PresignUploadTTL: "15m"}
			cfg.ApplyDefaults()
			client := &riskAttachmentHeadClient{b53StubClient: b53StubClient{info: &pkgstorage.ObjectInfo{Size: 10, ETag: "synthetic", ContentType: "image/png"}}}
			f.storage = storageservice.NewService(repository.NewStorageRepo(testPool), client, pkgstorage.NewConfigStore(cfg), slog.Default())
			f.storage.UseTicketAttachmentCommand(NewRiskWorkflowAttachmentCommand(f.svc, true))
			key, err := f.presign(t, f.actor, "ticket_attachment")
			require.NoError(t, err)
			heads := 0
			client.head = func() {
				heads++
				ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer cancel()
				require.NoError(t, repository.WithIAMGuard(ctx, testPool, func(ctx context.Context) error {
					q := repository.IAMDB(ctx, testPool)
					var err error
					switch change {
					case "epoch":
						_, err = q.Exec(ctx, `UPDATE users SET session_epoch=session_epoch+1 WHERE id=$1`, f.actor)
					case "finish_policy":
						_, err = q.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1='/api/v1/risk-items/remediation-finish'`, "role::"+f.role)
					case "reassigned":
						_, err = q.Exec(ctx, `UPDATE tickets SET assigned_to=$2 WHERE id=$1`, f.ticket, f.viewer)
					case "closed_group":
						_, err = q.Exec(ctx, `UPDATE risk_dispatch_groups SET ended_at=NOW() WHERE id=$1`, f.group)
					}
					return err
				}))
			}
			err = f.request(t, f.actor, "/api/v1/tickets/attachments/confirm", nil, func(ctx context.Context) error {
				_, err := f.storage.Confirm(ctx, &model.ConfirmFileInput{ObjectKey: key, TicketID: &f.ticket}, f.actor)
				return err
			})
			require.Error(t, err)
			require.Equal(t, 1, heads)
			var status string
			var n int
			require.NoError(t, testPool.QueryRow(context.Background(), `SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, f.role, key).Scan(&status))
			require.Equal(t, "pending", status)
			require.NoError(t, testPool.QueryRow(context.Background(), `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, f.ticket).Scan(&n))
			require.Zero(t, n)
		})
	}
}

func TestRiskWorkflowAttachmentReservationCannotChangeTicket(t *testing.T) {
	for _, enabled := range []bool{true, false} {
		t.Run(strconv.FormatBool(enabled), func(t *testing.T) {
			f := newRiskAttachmentFixture(t)
			key, err := f.presign(t, f.actor, "ticket_attachment")
			require.NoError(t, err)
			// A second ticket grants ordinary creator-update, which must not override pin.
			var other int64
			require.NoError(t, testPool.QueryRow(context.Background(), `INSERT INTO tickets(type_code,title,org_id,org_path,created_by,definition_version) SELECT type_code,'synthetic other',org_id,org_path,$2,definition_version FROM tickets WHERE id=$1 RETURNING id`, f.ticket, f.actor).Scan(&other))
			f.storage.UseTicketAttachmentCommand(NewRiskWorkflowAttachmentCommand(f.svc, enabled))
			err = f.request(t, f.actor, "/api/v1/tickets/attachments/confirm", nil, func(ctx context.Context) error {
				_, err := f.storage.Confirm(ctx, &model.ConfirmFileInput{ObjectKey: key, TicketID: &other}, f.actor)
				return err
			})
			require.Error(t, err)
		})
	}
}

func TestRiskWorkflowAttachmentUsesActualPublication(t *testing.T) {
	for _, change := range []string{"draft_only", "gate_off", "condition", "rejected"} {
		t.Run(change, func(t *testing.T) {
			f := newRiskAttachmentFixture(t)
			ctx := context.Background()
			if change == "rejected" {
				_, err := testPool.Exec(ctx, `UPDATE risk_ticket_links SET remediation_state='rejected' WHERE group_id=$1`, f.group)
				require.NoError(t, err)
				var sibling int64
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by) SELECT human_key||'-resolved',family_id,'closed',org_id,owner_user_id,title,created_by FROM risk_items WHERE id=$1 RETURNING id`, f.risk).Scan(&sibling))
				_, err = testPool.Exec(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link,remediation_state,resolution,resolved_risk_revision,effective_owner_id,effective_at) VALUES($1,$2,$3,'resolved','fixed',1,$3,NOW())`, sibling, f.group, f.creator)
				require.NoError(t, err)
			} else {
				update := &model.UpdateTicketTypeRequest{}
				if change == "condition" {
					update.TransitionMeta = rawJSON(`{"in_progress>pending_verify":{"roles":["creator"]}}`)
				} else {
					update.Transitions = rawJSON(`{"open":["assigned"],"assigned":["in_progress"],"in_progress":["pending_verify"],"pending_verify":["closed","in_progress"],"closed":["open"]}`)
				}
				_, err := f.svc.UpdateTicketType(ctx, f.role, update)
				require.NoError(t, err)
				if change != "draft_only" {
					def := publishD01(t, f.svc, f.role, f.creator)
					_, err = testPool.Exec(ctx, `UPDATE tickets SET definition_version=$2 WHERE id=$1`, f.ticket, def.DefinitionVersion)
					require.NoError(t, err)
					_, err = testPool.Exec(ctx, `UPDATE risk_dispatch_groups SET definition_version=$2 WHERE id=$1`, f.group, def.DefinitionVersion)
					require.NoError(t, err)
				}
			}
			_, err := f.presign(t, f.actor, "ticket_attachment")
			if change == "draft_only" || change == "rejected" {
				require.NoError(t, err)
			} else {
				require.Error(t, err)
			}
		})
	}
}

type riskAttachmentTrace struct {
	group chan struct{}
	once  sync.Once
}

func (tr *riskAttachmentTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	if strings.Contains(data.SQL, "SELECT id FROM risk_dispatch_groups WHERE id=") && strings.Contains(data.SQL, "FOR SHARE") {
		tr.once.Do(func() { close(tr.group) })
	}
	return ctx
}
func (*riskAttachmentTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func TestRiskWorkflowAttachmentConfirmAndAppendLockOrderSingleConnection(t *testing.T) {
	f := newRiskAttachmentFixture(t)
	key, err := f.presign(t, f.actor, "ticket_attachment")
	require.NoError(t, err)
	config := testPool.Config().Copy()
	config.MaxConns = 1
	trace := &riskAttachmentTrace{group: make(chan struct{})}
	config.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(context.Background(), config)
	require.NoError(t, err)
	defer pool.Close()
	delegation := orgdeleg.NewOrgDelegationService(pool)
	svc := NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool), resource.NewRegistry(), stubRoleFetcher{f.actor: {f.role}}, delegation, repository.NewStorageRepo(pool), nil, nil)
	cfg := &pkgstorage.StorageConfig{Enabled: true, Bucket: f.role, PresignUploadTTL: "15m"}
	cfg.ApplyDefaults()
	storage := storageservice.NewService(repository.NewStorageRepo(pool), &b53StubClient{info: &pkgstorage.ObjectInfo{Size: 10, ETag: "synthetic", ContentType: "image/png"}}, pkgstorage.NewConfigStore(cfg), slog.Default())
	storage.UseTicketAttachmentCommand(NewRiskWorkflowAttachmentCommand(svc, true))
	var admitted context.Context
	require.NoError(t, f.request(t, f.actor, "/api/v1/tickets/attachments/confirm", nil, func(ctx context.Context) error { admitted = ctx; return nil }))
	ctx, cancel := context.WithTimeout(admitted, 8*time.Second)
	defer cancel()
	// Use the dispatch append predecessor order on real rows. While holding the
	// group, wait until confirm requests that group, then prove ticket is free.
	appendTx, err := testPool.Begin(ctx)
	require.NoError(t, err)
	defer appendTx.Rollback(ctx)
	_, err = appendTx.Exec(ctx, `SELECT id FROM risk_dispatch_groups WHERE id=$1 FOR UPDATE`, f.group)
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() {
		_, err := storage.Confirm(ctx, &model.ConfirmFileInput{ObjectKey: key, TicketID: &f.ticket}, f.actor)
		done <- err
	}()
	select {
	case <-trace.group:
	case <-ctx.Done():
		t.Fatal("confirm did not reach predecessor group lock")
	}
	_, err = appendTx.Exec(ctx, `SET LOCAL lock_timeout='1s'`)
	require.NoError(t, err)
	_, err = appendTx.Exec(ctx, `SELECT id FROM risk_items WHERE id=$1 FOR UPDATE`, f.risk)
	require.NoError(t, err)
	_, err = appendTx.Exec(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, f.ticket)
	require.NoError(t, err, "confirm must not hold ticket while waiting for group")
	// A new member committed before confirm obtains group invalidates its
	// observation. No file may be confirmed using an incomplete risk scope.
	var risk int64
	require.NoError(t, appendTx.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by) SELECT human_key||'-append',family_id,status,org_id,owner_user_id,title,created_by FROM risk_items WHERE id=$1 RETURNING id`, f.risk).Scan(&risk))
	_, err = appendTx.Exec(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3)`, risk, f.group, f.creator)
	require.NoError(t, err)
	require.NoError(t, appendTx.Commit(ctx))
	require.ErrorIs(t, <-done, errcode.ErrConcurrentModification)
	// Retry observes the whole new group. MaxConns=1 proves authorization,
	// ticket locking, reservation check and ConfirmAndBind take no second lease.
	_, err = storage.Confirm(ctx, &model.ConfirmFileInput{ObjectKey: key, TicketID: &f.ticket}, f.actor)
	require.NoError(t, err)
	require.Zero(t, pool.Stat().CanceledAcquireCount())
	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, f.ticket).Scan(&n))
	require.Equal(t, 1, n)
}

func TestRiskWorkflowAttachmentCannotPromoteCreatePurpose(t *testing.T) {
	for _, purpose := range []string{"avatar", "attachment_create", "richtext_create"} {
		t.Run(purpose, func(t *testing.T) {
			f := newRiskAttachmentFixture(t)
			key, err := f.presign(t, f.actor, purpose)
			require.NoError(t, err)
			err = f.request(t, f.actor, "/api/v1/tickets/attachments/confirm", nil, func(ctx context.Context) error {
				_, err := f.storage.Confirm(ctx, &model.ConfirmFileInput{ObjectKey: key, TicketID: &f.ticket}, f.actor)
				return err
			})
			require.ErrorIs(t, err, errcode.ErrNoPermission, "non-ticket purpose with ticket_id must retain ordinary update preflight")
		})
	}
}

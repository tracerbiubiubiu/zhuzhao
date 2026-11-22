//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/channelsender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/msgrender"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type deliveryCaptureSender struct {
	mu     sync.Mutex
	counts map[string]int
}

func (s *deliveryCaptureSender) Channel() string { return "uip" }
func (s *deliveryCaptureSender) Send(_ context.Context, m channelsender.Message) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, id := range m.Recipients {
		s.counts[id]++
	}
	return nil
}
func deliveryDispatcher(t *testing.T, action, body string) (*MessageDispatcher, *deliveryCaptureSender, string) {
	t.Helper()
	code := "delivery_" + uniqueSuffix()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `INSERT INTO message_templates(code,name,channel,biz_type,subject,body,enabled)VALUES($1,$1,'uip','ticket','subject',$2,true)`, code, body)
	require.NoError(t, err)
	biz := "delivery_" + uniqueSuffix()
	_, err = testPool.Exec(ctx, `INSERT INTO message_rules(biz_type,event_action,template_code,recipient_type,enabled)VALUES($1,$2,$3,'cc',true)`, biz, action, code)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM message_rules WHERE biz_type=$1`, biz)
		_, _ = testPool.Exec(ctx, `DELETE FROM message_templates WHERE code=$1`, code)
	})
	capture := &deliveryCaptureSender{counts: map[string]int{}}
	d := NewMessageDispatcher(repository.NewMessageTemplateRepo(testPool), repository.NewMessageRuleRepo(testPool), repository.NewMessageLogRepo(testPool), nil, capture, slog.Default(), nil)
	// Dispatcher intentionally accepts domain-agnostic events. Per-test biz keeps
	// real rule/template reads isolated from seeded ticket rules.
	return d, capture, biz
}

func TestRecipientThrottleConcurrentOverlapEightRounds(t *testing.T) {
	d, capture, domain := deliveryDispatcher(t, "overlap", "body")
	biz := domainevent.BizType(domain)
	for round := 0; round < 8; round++ {
		t.Run(strconv.Itoa(round), func(t *testing.T) {
			var wg sync.WaitGroup
			start := make(chan struct{})
			ids := []string{fmt.Sprintf("%d1", round+10), fmt.Sprintf("%d2", round+10), fmt.Sprintf("%d3", round+10)}
			for _, recipients := range []string{ids[0] + "," + ids[1], ids[1] + "," + ids[2]} {
				wg.Add(1)
				go func(rcpt string) {
					defer wg.Done()
					<-start
					d.Dispatch(context.Background(), domainevent.DomainEvent{BizType: biz, BizID: fmt.Sprint(round), Action: "overlap", Payload: domainevent.Payload{"cc_users": rcpt}})
				}(recipients)
			}
			close(start)
			wg.Wait()
			capture.mu.Lock()
			defer capture.mu.Unlock()
			for _, id := range ids {
				require.Equal(t, 1, capture.counts[id], "each unseen recipient must be sent once, including partial overlap")
			}
		})
	}
}
func TestRenderRejectedPersistsFailedDiagnostic(t *testing.T) {
	d, capture, biz := deliveryDispatcher(t, "render_missing", "{{ticket.type_code}}")
	id := "missing-" + uniqueSuffix()
	d.Dispatch(context.Background(), domainevent.DomainEvent{BizType: domainevent.BizType(biz), BizID: id, Action: "render_missing", Payload: domainevent.Payload{"cc_users": "17"}})
	capture.mu.Lock()
	require.Empty(t, capture.counts)
	capture.mu.Unlock()
	var status, detail string
	require.NoError(t, testPool.QueryRow(context.Background(), `SELECT status,error FROM message_log WHERE biz_id=$1 ORDER BY id DESC LIMIT 1`, id).Scan(&status, &detail))
	require.Equal(t, "failed", status)
	require.Contains(t, detail, "ticket.type_code")
}
func TestUpdateFieldFileAndRichtextBindBeforeCommit(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := "fieldrefs_" + uniqueSuffix()
	_, err := env.svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	_, err = env.svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "docs", FieldLabel: "docs", FieldType: "file"}, {FieldKey: "body", FieldLabel: "body", FieldType: "richtext"}}})
	require.NoError(t, err)
	publishD01(t, env.svc, code, env.u1)
	req := env.createReq(nil)
	req.TypeCode = code
	tk, err := env.svc.Create(ctx, req, env.u1)
	require.NoError(t, err)
	file := env.confirmedCreate(t, env.u1, "attachment_create")
	image := env.confirmedCreate(t, env.u1, "richtext_create")
	raw, _ := json.Marshal(map[string]any{"docs": []string{file}, "body": "![](/api/v1/files/" + image + ")"})
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: raw}, env.u1)
	require.NoError(t, err)
	var n int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*)FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
	require.Equal(t, 2, n, "custom_data references must bind even when req.attachments is absent")
	for _, key := range []string{file, image} {
		var purpose, owner string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT purpose,owner_id FROM file_objects WHERE bucket='b53' AND object_key=$1`, key).Scan(&purpose, &owner))
		require.True(t, strings.HasPrefix(purpose, "ticket_"))
		require.Equal(t, strconv.FormatInt(tk.ID, 10), owner)
	}
	// A second legal editor can retain same-ticket references uploaded by u1.
	env.svc.roleFetcher.(stubRoleFetcher)[env.u2] = []string{"admin"}
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: raw}, env.u2)
	require.NoError(t, err)
}

func deliveryFileType(t *testing.T, env *b53Env) string {
	t.Helper()
	code := "refs_" + uniqueSuffix()
	_, err := env.svc.CreateTicketType(context.Background(), &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	_, err = env.svc.ReplaceTicketTypeFields(context.Background(), code, &model.ReplaceTypeFieldsRequest{Code: code, Fields: []model.TicketTypeFieldInput{{FieldKey: "docs", FieldLabel: "docs", FieldType: "file"}, {FieldKey: "body", FieldLabel: "body", FieldType: "richtext"}}})
	require.NoError(t, err)
	publishD01(t, env.svc, code, env.u1)
	return code
}
func deliveryFileTicket(t *testing.T, env *b53Env, code string) *model.Ticket {
	t.Helper()
	req := env.createReq(nil)
	req.TypeCode = code
	tk, err := env.svc.Create(context.Background(), req, env.u1)
	require.NoError(t, err)
	return tk
}
func TestUpdateFieldReferencesRejectInvalidAndRollBackWholePatch(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := deliveryFileType(t, env)
	for _, kind := range []string{"unknown-file", "unknown-richtext", "other-person", "pending", "deleted", "other-ticket", "wrong-file-purpose", "wrong-richtext-purpose"} {
		t.Run(kind, func(t *testing.T) {
			tk := deliveryFileTicket(t, env, code)
			key := "unknown-" + uniqueSuffix()
			purpose := "attachment_create"
			owner := env.u1
			field := "docs"
			switch kind {
			case "other-person":
				owner = env.u2
			case "wrong-file-purpose":
				purpose = "richtext_create"
			case "wrong-richtext-purpose":
				field = "body"
			case "unknown-richtext":
				field = "body"
			}
			if !strings.HasPrefix(kind, "unknown") {
				if kind == "pending" {
					response, err := env.stg.PresignUpload(ctx, &model.PresignUploadInput{Purpose: purpose, FileName: "p.png", MIMEType: "image/png", SizeBytes: 10}, owner)
					require.NoError(t, err)
					key = response.ObjectKey
				} else {
					key = env.confirmedCreate(t, owner, purpose)
				}
			}
			if kind == "deleted" {
				_, err := testPool.Exec(ctx, `UPDATE file_objects SET status='deleted' WHERE bucket='b53' AND object_key=$1`, key)
				require.NoError(t, err)
			}
			if kind == "other-ticket" {
				_, err := env.svc.Create(ctx, env.createReq([]string{key}), env.u1)
				require.NoError(t, err)
			}
			var value any = []string{key}
			if field == "body" {
				value = "![](/api/v1/files/" + key + ")"
			}
			raw, _ := json.Marshal(map[string]any{field: value})
			title := "must roll back"
			_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Title: &title, CustomData: raw}, env.u1)
			requireErrCode(t, err, 10001)
			saved, err := env.svc.ticketRepo.GetByID(ctx, tk.ID)
			require.NoError(t, err)
			require.Equal(t, tk.Title, saved.Title)
			require.JSONEq(t, `{}`, string(saved.CustomData))
			var n int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*)FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
			require.Zero(t, n)
		})
	}
	t.Run("cross source dedup but explicit duplicates remain invalid", func(t *testing.T) {
		tk := deliveryFileTicket(t, env, code)
		key := env.confirmedCreate(t, env.u1, "attachment_create")
		raw, _ := json.Marshal(map[string]any{"docs": []string{key, key}})
		_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: raw, Attachments: []string{key}}, env.u1)
		require.NoError(t, err)
		var n int
		require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*)FROM ticket_attachments WHERE ticket_id=$1`, tk.ID).Scan(&n))
		require.Equal(t, 1, n)
		fresh := env.confirmedCreate(t, env.u1, "attachment_create")
		_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, Attachments: []string{fresh, fresh}}, env.u1)
		requireErrCode(t, err, 10001)
	})
}
func TestUpdateFieldReferenceConcurrentClaimEightRounds(t *testing.T) {
	env := newB53Env(t)
	code := deliveryFileType(t, env)
	ctx := context.Background()
	for round := 0; round < 8; round++ {
		t.Run(strconv.Itoa(round), func(t *testing.T) {
			a := deliveryFileTicket(t, env, code)
			b := deliveryFileTicket(t, env, code)
			key := env.confirmedCreate(t, env.u1, "attachment_create")
			raw, _ := json.Marshal(map[string]any{"docs": []string{key}})
			start := make(chan struct{})
			results := make(chan error, 2)
			for _, id := range []int64{a.ID, b.ID} {
				go func(id int64) {
					<-start
					_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: id, CustomData: raw}, env.u1)
					results <- err
				}(id)
			}
			close(start)
			first, second := <-results, <-results
			wins := 0
			for _, err := range []error{first, second} {
				if err == nil {
					wins++
				} else {
					requireErrCode(t, err, 10001)
				}
			}
			require.Equal(t, 1, wins, "one confirmed dangling object cannot commit to two tickets")
			var n int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*)FROM ticket_attachments ta JOIN file_objects fo ON ta.file_object_id=fo.id WHERE fo.bucket='b53' AND fo.object_key=$1`, key).Scan(&n))
			require.Equal(t, 1, n)
		})
	}
}
func TestUpdateFieldReferencesUseTransactionConnectionEightRounds(t *testing.T) {
	env := newB53Env(t)
	code := deliveryFileType(t, env)
	cfg := testPool.Config().Copy()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	defer pool.Close()
	env.svc.db = pool
	env.svc.ticketRepo = repository.NewTicketRepo(pool)
	env.svc.storageRepo = repository.NewStorageRepo(pool)
	for round := 0; round < 8; round++ {
		t.Run(strconv.Itoa(round), func(t *testing.T) {
			tk := deliveryFileTicket(t, env, code)
			key := env.confirmedCreate(t, env.u1, "attachment_create")
			raw, _ := json.Marshal(map[string]any{"docs": []string{key}})
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			_, err := env.svc.Update(ctx, &model.UpdateTicketRequest{ID: tk.ID, CustomData: raw}, env.u1)
			require.NoError(t, err, "holding the transaction's only connection cannot require a second pool checkout")
		})
	}
}

func TestServiceEventSnapshotsHaveRealBuiltinValuesAndNewStatus(t *testing.T) {
	svc, creator, actor, assignee, roles := setupTicket2a(t)
	roles[creator] = []string{"admin"}
	roles[actor] = []string{"admin"}
	ctx := context.Background()
	code := "snapshot_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	publishD01(t, svc, code, creator)
	reg := domainevent.NewHookRegistry(nil)
	events := make(chan domainevent.DomainEvent, 8)
	reg.Subscribe(domainevent.BizTicket, "snapshot.capture", domainevent.HookFunc(func(_ context.Context, ev domainevent.DomainEvent) { events <- ev }))
	svc.SetEventRegistry(reg)
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		require.NoError(t, reg.Drain(ctx))
	}()
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "snapshot title", Priority: 2, OrgID: childOrgID(t, "snapshot")}, creator)
	require.NoError(t, err)
	require.NotEmpty(t, tk.HumanKey)
	require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &assignee}, actor))
	_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: "in_progress"}, actor)
	require.NoError(t, err)
	require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: tk.ID}, actor))
	expected := map[domainevent.Action]string{domainevent.TicketCreated: "open", domainevent.TicketAssigned: "assigned", domainevent.TicketClosed: "closed", domainevent.TicketTransitioned: "in_progress"}
	for i := 0; i < 4; i++ {
		select {
		case ev := <-events:
			require.Equal(t, expected[ev.Action], ev.Payload["status"])
			require.Equal(t, tk.HumanKey, ev.Payload["human_key"])
			rendered := msgrender.RenderQuadrant("{{ticket.title}} {{ticket.type_code}} {{ticket.priority}} {{ticket.status}} {{ticket.human_key}} {{ticket.link}}", buildEventVars(ev))
			require.NoError(t, rendered.Err)
			require.Contains(t, rendered.Text, tk.HumanKey)
		case <-time.After(2 * time.Second):
			t.Fatal("missing real service event")
		}
	}
}

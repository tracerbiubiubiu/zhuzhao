//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestD01InstanceUsesBoundRules(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_instance_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{"assigned":{"sla_due_hours":2}}`)})
	require.NoError(t, err)
	a := publishD01(t, svc, code, actor)
	create := func(customData string) *model.Ticket {
		t.Helper()
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, OrgID: org(), Title: "synthetic instance", CustomData: rawJSON(customData)}, actor)
		require.NoError(t, err)
		return tk
	}
	ta := create(`{}`)
	closeA := create(`{}`)
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{
		{FieldKey: "env", FieldLabel: "environment", FieldType: "input", Required: true},
	}})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{
		"assigned":{"sla_due_hours":5},
		"open>closed":{"roles":["assignee"]},
		"open>assigned":{"validator":"priority == 2"}
	}`)})
	require.NoError(t, err)
	b := publishD01(t, svc, code, actor)
	tb := create(`{"env":"test"}`)
	closeB := create(`{"env":"test"}`)
	// A later draft would make both sets of action rules and deadlines drift.
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{"assigned":{"sla_due_hours":9}}`)})
	require.NoError(t, err)

	t.Run("available", func(t *testing.T) {
		actionsA, err := svc.ListAvailableTransitions(ctx, ta.ID, actor)
		require.NoError(t, err)
		actionsB, err := svc.ListAvailableTransitions(ctx, tb.ID, actor)
		require.NoError(t, err)
		require.Contains(t, actionsA, StatusClosed)
		require.NotContains(t, actionsB, StatusClosed)
	})
	t.Run("fields", func(t *testing.T) {
		_, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: ta.ID, CustomData: rawJSON(`{}`)}, actor)
		require.NoError(t, err)
		_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tb.ID, CustomData: rawJSON(`{"env":null}`)}, actor)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	})
	t.Run("transition_and_sla", func(t *testing.T) {
		_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: ta.ID, To: StatusAssigned}, actor)
		require.NoError(t, err)
		err = svc.Assign(ctx, &model.AssignTicketRequest{ID: tb.ID, AssignedTo: &actor}, actor)
		requireErrCode(t, err, errcode.ErrInvalidParams.Code)
		_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: tb.ID, Priority: intPtr(2)}, actor)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tb.ID, AssignedTo: &actor}, actor))
		for _, tc := range []struct {
			ticket *model.Ticket
			def    *model.TicketTypeDefinition
			hours  time.Duration
		}{{ta, a, 2}, {tb, b, 5}} {
			read, err := svc.Get(ctx, tc.ticket.ID, actor)
			require.NoError(t, err)
			require.Equal(t, tc.hours*time.Hour, read.SLADueAt.Sub(*read.StatusChangedAt))
			assertD01InstanceWire(t, read, "exact", tc.def)
		}
	})
	t.Run("close", func(t *testing.T) {
		require.NoError(t, svc.Close(ctx, &model.CloseTicketRequest{ID: closeA.ID}, actor))
		requireErrCode(t, svc.Close(ctx, &model.CloseTicketRequest{ID: closeB.ID}, actor), errcode.ErrNoPermission.Code)
		after, err := svc.Get(ctx, closeB.ID, actor)
		require.NoError(t, err)
		require.Equal(t, StatusOpen, after.Status)
	})
}

func TestD01InstanceDegradedReadAndZeroWrites(t *testing.T) {
	svc, actor, _, outsider, roles := setupTicket2a(t)
	roles[actor] = []string{"admin"}
	ctx := context.Background()
	org := rootOrgID(t)
	code := "d01_degraded_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	def := publishD01(t, svc, code, actor)
	_, err = testPool.Exec(ctx, `INSERT INTO ticket_type_definitions
		(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by)
		SELECT type_code,99,definition_hash,99,snapshot,source_draft_revision,$2
		FROM ticket_type_definitions WHERE type_code=$1 AND definition_version=$3`, code, actor, def.DefinitionVersion)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO ticket_type_definitions
		(type_code,definition_version,definition_hash,schema_format_version,snapshot,source_draft_revision,published_by)
		SELECT type_code,98,ticket_definition_snapshot_hash(invalid_graph),1,invalid_graph,source_draft_revision,$2
		FROM (SELECT type_code,source_draft_revision,jsonb_set(snapshot,'{type,states}','["new","done"]') AS invalid_graph
			FROM ticket_type_definitions WHERE type_code=$1 AND definition_version=$3) AS legacy_definition`, code, actor, def.DefinitionVersion)
	require.NoError(t, err)

	unknownFormat, invalidGraph := int64(99), int64(98)
	for _, tc := range []struct {
		name    string
		version *int64
	}{{"unbound", nil}, {"unknown_format", &unknownFormat}, {"invalid_graph", &invalidGraph}} {
		t.Run(tc.name, func(t *testing.T) {
			var id int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets
				(type_code, title, status, created_by, org_id, org_path, status_changed_at, sla_due_at, definition_version)
				SELECT $1,'synthetic legacy','assigned',$2,id,path,'2020-01-02 03:00:00Z','2020-01-03 03:00:00Z',$4
				FROM organizations WHERE id=$3 RETURNING id`, code, actor, org, tc.version).Scan(&id))
			_, err := testPool.Exec(ctx, `INSERT INTO ticket_comments(ticket_id,user_id,content,is_internal)
				VALUES ($1,$2,'original history',false)`, id, actor)
			require.NoError(t, err)
			_, err = testPool.Exec(ctx, `INSERT INTO ticket_events(ticket_id,user_id,action) VALUES ($1,$2,'created')`, id, actor)
			require.NoError(t, err)
			before, err := svc.ticketRepo.GetByID(ctx, id)
			require.NoError(t, err)
			t.Run("read", func(t *testing.T) {
				read, err := svc.Get(ctx, id, actor)
				require.NoError(t, err)
				require.Equal(t, before.StatusChangedAt, read.StatusChangedAt)
				require.Equal(t, before.SLADueAt, read.SLADueAt)
				require.Equal(t, tc.version, read.DefinitionVersion)
				assertD01InstanceWire(t, read, "degraded", nil)
				actions, err := svc.ListAvailableTransitions(ctx, id, actor)
				require.NoError(t, err)
				require.Equal(t, []string{}, actions)
				history, err := svc.ListComments(ctx, id, actor)
				require.NoError(t, err)
				require.Len(t, history, 1)
				require.Equal(t, "original history", history[0].Content)
			})
			title := "rejected edit"
			t.Run("writes", func(t *testing.T) {
				called := false
				for action, attempt := range map[string]func() error{
					"title": func() error {
						_, err := svc.Update(ctx, &model.UpdateTicketRequest{ID: id, Title: &title}, actor)
						return err
					},
					"transition": func() error {
						_, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: id, To: StatusInProgress}, actor)
						return err
					},
					"assign": func() error { return svc.Assign(ctx, &model.AssignTicketRequest{ID: id, AssignedTo: &actor}, actor) },
					"automatic_assign": func() error {
						return svc.assignAsSystem(ctx, &model.AssignTicketRequest{ID: id, AssignedTo: &actor}, actor)
					},
					"close":  func() error { return svc.Close(ctx, &model.CloseTicketRequest{ID: id}, actor) },
					"delete": func() error { return svc.Delete(ctx, id, actor) },
					"note": func() error {
						_, err := svc.CreateNote(ctx, &model.CreateNoteRequest{TicketID: id, Content: "rejected note"}, actor)
						return err
					},
					"cc_add": func() error {
						_, err := svc.AddCC(ctx, &model.AddTicketCCRequest{TicketID: id, UserIDs: []int64{outsider}}, actor)
						return err
					},
					"tag_add": func() error {
						_, err := svc.AddTags(ctx, &model.AddTicketTagsRequest{TicketID: id, Tags: []string{"rejected"}}, actor)
						return err
					},
					"attachment": func() error {
						return svc.AttachmentCommand(ctx, "confirm", actor, id, func(context.Context) error { called = true; return nil })
					},
					"comment": func() error {
						_, err := svc.CreateComment(ctx, &model.CreateCommentRequest{TicketID: id, Content: "rejected history"}, actor)
						return err
					},
				} {
					t.Run(action, func(t *testing.T) { requireErrCode(t, attempt(), errcode.ErrServiceUnavailable.Code) })
				}
				require.False(t, called)
			})
			t.Run("authorization", func(t *testing.T) {
				_, err := svc.Get(ctx, id, outsider)
				requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
				_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: id, Title: &title}, outsider)
				requireErrCode(t, err, errcode.ErrTicketNotFound.Code)
				// A CC member can see the ticket but cannot update it.
				_, err = svc.ccRepo.Add(ctx, id, actor, []int64{outsider})
				require.NoError(t, err)
				_, err = svc.Update(ctx, &model.UpdateTicketRequest{ID: id, Title: &title}, outsider)
				requireErrCode(t, err, errcode.ErrNoPermission.Code)
			})
			t.Run("preservation", func(t *testing.T) {
				after, err := svc.ticketRepo.GetByID(ctx, id)
				require.NoError(t, err)
				require.Equal(t, before, after)
				var events, comments int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT (SELECT count(*) FROM ticket_events WHERE ticket_id=$1),
					(SELECT count(*) FROM ticket_comments WHERE ticket_id=$1)`, id).Scan(&events, &comments))
				require.Equal(t, 1, events)
				require.Equal(t, 1, comments)
			})
		})
	}
}

func TestD01AttachmentDeleteUsesBoundFields(t *testing.T) {
	env := newB53Env(t)
	ctx := context.Background()
	code := deliveryFileType(t, env)
	key := env.confirmedCreate(t, env.u1, "attachment_create")
	req := env.createReq(nil)
	req.TypeCode = code
	req.CustomData = c02Data(t, map[string]any{"docs": []string{key}})
	ticket, err := env.svc.Create(ctx, req, env.u1)
	require.NoError(t, err)
	// Simulate a mismatching draft left by an external repair. The stored
	// ticket field is still a file under the original definition.
	_, err = testPool.Exec(ctx, `UPDATE ticket_type_fields SET field_type='input' WHERE type_code=$1 AND field_key='docs'`, code)
	require.NoError(t, err)
	requireErrCode(t, env.stg.DeleteTicketAttachment(ctx, ticket.ID, key), errcode.ErrConflict.Code)
	_, err = env.svc.Update(ctx, &model.UpdateTicketRequest{ID: ticket.ID, CustomData: rawJSON(`{"docs":null}`)}, env.u1)
	require.NoError(t, err)
	require.NoError(t, env.stg.DeleteTicketAttachment(ctx, ticket.ID, key))
}

func TestD01TimeoutScannerUsesBoundVersion(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_timeout_" + uniqueSuffix()
	auditRetireReminderType(t, code)
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	setTimeout := func(meta string) {
		t.Helper()
		_, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(meta)})
		require.NoError(t, err)
	}
	create := func() *model.Ticket {
		t.Helper()
		tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, OrgID: org(), Title: "synthetic timeout"}, actor)
		require.NoError(t, err)
		require.NoError(t, svc.Assign(ctx, &model.AssignTicketRequest{ID: tk.ID, AssignedTo: &actor}, actor))
		backdateStatusChanged(t, tk.ID, 2*time.Hour)
		return tk
	}
	setTimeout(`{"assigned":{"timeout_minutes":60,"sla_due_hours":1}}`)
	publishD01(t, svc, code, actor)
	a := create()
	_, err = testPool.Exec(ctx, `UPDATE tickets SET sla_due_at=NOW()-interval '1 hour' WHERE id=$1`, a.ID)
	require.NoError(t, err)
	setTimeout(`{"assigned":{"timeout_minutes":180}}`)
	publishD01(t, svc, code, actor)
	b := create()
	setTimeout(`{"assigned":{"timeout_minutes":30}}`)
	scanner := NewTimeoutScanner(testPool, domainevent.NewHookRegistry(nil), false, nil)
	require.NoError(t, scanner.Handle(ctx, nil))
	for _, tc := range []struct {
		id   int64
		want []string
	}{{a.ID, []string{"assigned", "deadline:assigned"}}, {b.ID, nil}} {
		var states []string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT array_agg(state ORDER BY state) FROM ticket_reminders WHERE ticket_id=$1`, tc.id).Scan(&states))
		require.Equal(t, tc.want, states)
	}
}

func assertD01InstanceWire(t *testing.T, ticket *model.Ticket, resolution string, def *model.TicketTypeDefinition) {
	t.Helper()
	b, err := json.Marshal(ticket)
	require.NoError(t, err)
	var view map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(b, &view))
	require.JSONEq(t, `"`+resolution+`"`, string(view["definition_resolution"]))
	if def == nil {
		require.JSONEq(t, "null", string(view["definition_ref"]))
		return
	}
	require.JSONEq(t, `{"type_code":"`+def.TypeCode+`","definition_version":"`+fmtInt(def.DefinitionVersion)+`","definition_hash":"`+def.DefinitionHash+`"}`, string(view["definition_ref"]))
}

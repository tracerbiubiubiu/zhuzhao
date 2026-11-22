//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestD02PinnedEvidenceAndTransitionCommitTogether(t *testing.T) {
	ctx := context.Background()
	svc, actor, org := setupTicketAdmin(t)
	code := "d02_joint_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	fields := []model.TicketTypeFieldInput{
		{FieldKey: "repair", FieldLabel: "整改说明", FieldType: "input"},
		{FieldKey: "evidence", FieldLabel: "整改附件", FieldType: "file"},
	}
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: fields})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{
		States:      rawJSON(`["open","assigned","in_progress","review","closed"]`),
		Transitions: rawJSON(`{"open":["review"],"review":["closed"]}`),
		TransitionMeta: rawJSON(`{"open>review":{"collect_fields":[
			{"key":"repair","required":true},{"key":"evidence","required":true}
		]}}`),
	})
	require.NoError(t, err)
	a := publishD01(t, svc, code, actor)
	create := func() *model.Ticket {
		t.Helper()
		record, err := svc.Create(ctx, &model.CreateTicketRequest{
			TypeCode: code, OrgID: org(), Title: "Synthetic D02 joint acceptance",
		}, actor)
		require.NoError(t, err)
		return record
	}
	ta := create()
	fields = append(fields, model.TicketTypeFieldInput{
		FieldKey: "environment", FieldLabel: "复测环境", FieldType: "input",
	})
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: fields})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{
		TransitionMeta: rawJSON(`{"open>review":{"collect_fields":[
			{"key":"repair","required":true},{"key":"evidence","required":true},{"key":"environment","required":true}
		]}}`),
	})
	require.NoError(t, err)
	require.Equal(t, a.DefinitionVersion, *create().DefinitionVersion)
	b := publishD01(t, svc, code, actor)
	_, err = svc.Create(ctx, &model.CreateTicketRequest{
		TypeCode: code, OrgID: org(), Title: "stale preview",
		DefinitionVersion: strconv.FormatInt(a.DefinitionVersion, 10), DefinitionHash: a.DefinitionHash,
	}, actor)
	requireErrCode(t, err, errcode.ErrConcurrentModification.Code)
	tb := create()
	require.Equal(t, a.DefinitionVersion, *ta.DefinitionVersion)
	require.Equal(t, b.DefinitionVersion, *tb.DefinitionVersion)
	require.NotEqual(t, a.DefinitionHash, b.DefinitionHash)

	initAction := func(record *model.Ticket, key string, data map[string]any) *model.TransitionTicketRequest {
		t.Helper()
		payload, err := json.Marshal(data)
		require.NoError(t, err)
		return &model.TransitionTicketRequest{
			ID: record.ID, To: "review", IdempotencyKey: key,
			ExpectedRevision:  rawJSON(strconv.Quote(strconv.FormatInt(record.Revision, 10))),
			DefinitionVersion: rawJSON(strconv.Quote(strconv.FormatInt(*record.DefinitionVersion, 10))),
			CustomDataPatch:   payload,
		}
	}
	type facts struct {
		status string
		data   string
		rev    int64
		links  int
		events int
	}
	read := func(id int64) facts {
		t.Helper()
		var f facts
		require.NoError(t, testPool.QueryRow(ctx, `SELECT status, custom_data::text, revision,
			(SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1),
			(SELECT count(*) FROM ticket_events WHERE ticket_id=$1)
			FROM tickets WHERE id=$1`, id).Scan(&f.status, &f.data, &f.rev, &f.links, &f.events))
		return f
	}
	collect := func(record *model.Ticket, count int) {
		t.Helper()
		actions, err := svc.ListAvailableActions(ctx, record.ID, actor)
		require.NoError(t, err)
		require.Len(t, actions.Actions, 1)
		require.Equal(t, record.Revision, actions.Actions[0].ExpectedRevision)
		require.Len(t, actions.Actions[0].CollectFields, count)
	}
	collect(ta, 2)
	collect(tb, 3)
	fileA, fileB := "synthetic/d02/a-"+uniqueSuffix(), "synthetic/d02/b-"+uniqueSuffix()
	seedPendingObject(t, actor, fileA, "image/png")
	seedPendingObject(t, actor, fileB, "image/png")
	beforeA := read(ta.ID)
	reqA := initAction(ta, "ka-"+uniqueSuffix(), map[string]any{"repair": "A", "evidence": []string{fileA}})
	afterA, err := svc.Transition(ctx, reqA, actor)
	require.NoError(t, err)
	require.Equal(t, "review", afterA.Status)
	committedA := read(ta.ID)
	require.Equal(t, beforeA.links+1, committedA.links)
	require.Equal(t, beforeA.events+1, committedA.events)
	require.Equal(t, beforeA.rev+1, committedA.rev)
	require.JSONEq(t, fmt.Sprintf(`{"repair":"A","evidence":[%q]}`, fileA), committedA.data)
	replay, err := svc.Transition(ctx, reqA, actor)
	require.NoError(t, err)
	require.Equal(t, afterA.Revision, replay.Revision)
	require.Equal(t, committedA, read(ta.ID))

	beforeB := read(tb.ID)
	_, err = svc.Transition(ctx, initAction(tb, "missing-"+uniqueSuffix(),
		map[string]any{"repair": "B", "evidence": []string{fileB}}), actor)
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	require.Equal(t, beforeB, read(tb.ID))
	_, err = svc.Transition(ctx, initAction(tb, "invalid-"+uniqueSuffix(),
		map[string]any{"repair": "B", "environment": "test", "evidence": []string{"synthetic/missing.png"}}), actor)
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	require.Equal(t, beforeB, read(tb.ID))
	_, err = svc.Transition(ctx, &model.TransitionTicketRequest{ID: tb.ID, To: "review"}, actor)
	requireErrCode(t, err, errcode.ErrInvalidParams.Code)
	require.Equal(t, beforeB, read(tb.ID))

	name := pgx.Identifier{"d02_reject_event_" + strconv.FormatInt(tb.ID, 10)}.Sanitize()
	_, err = testPool.Exec(ctx, fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN IF NEW.ticket_id = %d AND NEW.action = 'status_changed' THEN
		  RAISE EXCEPTION 'synthetic D02 commit failure'; END IF; RETURN NEW; END $$;
		CREATE TRIGGER %s BEFORE INSERT ON ticket_events FOR EACH ROW EXECUTE FUNCTION %s()`, name, tb.ID, name, name))
	require.NoError(t, err)
	removeFault := func() {
		_, _ = testPool.Exec(ctx, fmt.Sprintf("DROP TRIGGER IF EXISTS %s ON ticket_events; DROP FUNCTION IF EXISTS %s()", name, name))
	}
	t.Cleanup(removeFault)
	reqB := initAction(tb, "kb-"+uniqueSuffix(),
		map[string]any{"repair": "B", "environment": "test", "evidence": []string{fileB}})
	_, err = svc.Transition(ctx, reqB, actor)
	require.ErrorContains(t, err, "synthetic D02 commit failure")
	require.Equal(t, beforeB, read(tb.ID))
	removeFault()
	afterB, err := svc.Transition(ctx, reqB, actor)
	require.NoError(t, err)
	committedB := read(tb.ID)
	require.Equal(t, "review", afterB.Status)
	require.Equal(t, beforeB.links+1, committedB.links)
	require.Equal(t, beforeB.events+1, committedB.events)
	require.Equal(t, beforeB.rev+1, committedB.rev)
	require.JSONEq(t, fmt.Sprintf(`{"repair":"B","environment":"test","evidence":[%q]}`, fileB), committedB.data)
}

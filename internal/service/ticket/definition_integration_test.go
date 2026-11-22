//go:build integration

package ticket

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestD01DraftCannotCreate(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_draft_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	ticket, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "synthetic draft", OrgID: org()}, actor)
	require.Error(t, err, "saving a draft must not publish it or permit create")
	require.Nil(t, ticket)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE type_code=$1`, code).Scan(&count))
	require.Zero(t, count)
}

func publishD01(t *testing.T, svc *Service, code string, actor int64) *model.TicketTypeDefinition {
	t.Helper()
	draft, err := svc.ticketRepo.GetTicketType(context.Background(), code)
	require.NoError(t, err)
	def, changed, err := svc.PublishTicketType(context.Background(), draft, actor)
	require.NoError(t, err)
	require.True(t, changed)
	return def
}

func TestD01PublishedCreateIgnoresDraft(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_pub_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{{FieldKey: "plan", FieldLabel: "plan", FieldType: "input", Required: true}}})
	require.NoError(t, err)
	a := publishD01(t, svc, code, actor)
	require.EqualValues(t, 1, a.DefinitionVersion)
	// Saving a stricter draft does not change A's create contract.
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{{FieldKey: "plan", FieldLabel: "plan", FieldType: "input", Required: true}, {FieldKey: "env", FieldLabel: "env", FieldType: "input", Required: true}}})
	require.NoError(t, err)
	create := func(data string) (*model.Ticket, error) {
		return svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "synthetic published", OrgID: org(), CustomData: rawJSON(data)}, actor)
	}
	ta, err := create(`{"plan":"fix"}`)
	require.NoError(t, err)
	require.NotNil(t, ta.DefinitionVersion)
	require.Equal(t, a.DefinitionVersion, *ta.DefinitionVersion)
	b := publishD01(t, svc, code, actor)
	require.EqualValues(t, 2, b.DefinitionVersion)
	_, err = create(`{"plan":"fix"}`)
	require.Error(t, err, "B requires env")
	tb, err := create(`{"plan":"fix","env":"test"}`)
	require.NoError(t, err)
	require.Equal(t, b.DefinitionVersion, *tb.DefinitionVersion)
	read, err := svc.Get(ctx, ta.ID, actor)
	require.NoError(t, err)
	require.Equal(t, ta.DefinitionVersion, read.DefinitionVersion)
}

func TestD01PublicationCASAndABA(t *testing.T) {
	svc, actor, _ := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_aba_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	stale, err := svc.ticketRepo.GetTicketType(ctx, code)
	require.NoError(t, err)
	a := publishD01(t, svc, code, actor)
	// Pointer change alone must conflict even when draft revision is unchanged.
	_, _, err = svc.PublishTicketType(ctx, stale, actor)
	require.Error(t, err)
	fresh, err := svc.ticketRepo.GetTicketType(ctx, code)
	require.NoError(t, err)
	same, changed, err := svc.PublishTicketType(ctx, fresh, actor)
	require.NoError(t, err)
	require.False(t, changed)
	require.Equal(t, a.DefinitionVersion, same.DefinitionVersion)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{"open":{"sla_due_hours":2}}`)})
	require.NoError(t, err)
	_, _, err = svc.PublishTicketType(ctx, fresh, actor)
	require.Error(t, err, "stale draft must conflict")
	b := publishD01(t, svc, code, actor)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{}`)})
	require.NoError(t, err)
	back := publishD01(t, svc, code, actor)
	require.EqualValues(t, 3, back.DefinitionVersion)
	require.Equal(t, a.DefinitionHash, back.DefinitionHash, "A-B-A must be allowed")
	require.NotEqual(t, b.DefinitionHash, back.DefinitionHash)
}

func TestD01DefinitionsImmutableAndPublicationAtomic(t *testing.T) {
	svc, actor, _ := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_atomic_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	a := publishD01(t, svc, code, actor)
	for _, q := range []string{`UPDATE ticket_type_definitions SET snapshot='{}' WHERE type_code=$1`, `DELETE FROM ticket_type_definitions WHERE type_code=$1`} {
		_, err = testPool.Exec(ctx, q, code)
		require.Error(t, err, "published definitions are immutable")
	}
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: rawJSON(`{"open":{"sla_due_hours":4}}`)})
	require.NoError(t, err)
	// Fail the pointer write after the definition insert, using a type-specific constraint.
	constraint := "d01_fail_" + uniqueSuffix()
	_, err = testPool.Exec(ctx, `ALTER TABLE ticket_types ADD CONSTRAINT `+constraint+` CHECK (code <> '`+code+`' OR current_published_version = 1)`)
	require.NoError(t, err)
	t.Cleanup(func() { _, _ = testPool.Exec(ctx, `ALTER TABLE ticket_types DROP CONSTRAINT `+constraint) })
	draft, err := svc.ticketRepo.GetTicketType(ctx, code)
	require.NoError(t, err)
	def, changed, err := svc.PublishTicketType(ctx, draft, actor)
	require.Error(t, err)
	require.Nil(t, def)
	require.False(t, changed)
	var count, version int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_type_definitions WHERE type_code=$1`, code).Scan(&count))
	require.EqualValues(t, 1, count)
	require.NoError(t, testPool.QueryRow(ctx, `SELECT current_published_version FROM ticket_types WHERE code=$1`, code).Scan(&version))
	require.Equal(t, a.DefinitionVersion, version)
}

func TestD01StableFieldIdentityAndABA(t *testing.T) {
	svc, actor, _ := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_fields_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	fields := []model.TicketTypeField{{FieldKey: "plan", FieldLabel: "plan", FieldType: "input", FieldOptions: rawJSON(`[]`), FieldMeta: rawJSON(`{}`)}}
	require.NoError(t, svc.ticketRepo.ReplaceTypeFields(ctx, code, fields, nil))
	a := publishD01(t, svc, code, actor)
	old, err := svc.ticketRepo.ListTicketTypeFields(ctx, code)
	require.NoError(t, err)
	require.Positive(t, old[0].FieldID)
	fields[0].FieldLabel = "renamed"
	require.NoError(t, svc.ticketRepo.ReplaceTypeFields(ctx, code, fields, nil))
	b := publishD01(t, svc, code, actor)
	now, err := svc.ticketRepo.ListTicketTypeFields(ctx, code)
	require.NoError(t, err)
	require.NotEqual(t, old[0].ID, now[0].ID)
	require.Equal(t, old[0].FieldID, now[0].FieldID)
	fields[0].FieldLabel = "plan"
	require.NoError(t, svc.ticketRepo.ReplaceTypeFields(ctx, code, fields, nil))
	back := publishD01(t, svc, code, actor)
	require.Equal(t, a.DefinitionHash, back.DefinitionHash)
	require.NotEqual(t, b.DefinitionHash, back.DefinitionHash)
	// Storage-level retirement test; the public draft editor retains its existing hard lock.
	require.NoError(t, svc.ticketRepo.ReplaceTypeFields(ctx, code, nil, nil))
	require.NoError(t, svc.ticketRepo.ReplaceTypeFields(ctx, code, fields, nil))
	reused, err := svc.ticketRepo.ListTicketTypeFields(ctx, code)
	require.NoError(t, err)
	require.NotEqual(t, old[0].FieldID, reused[0].FieldID)
}

func TestD01CreateRollbackAndCurrentOrgGuard(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_createfail_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	publishD01(t, svc, code, actor)
	// A post-insert attachment error must roll back the bound ticket and event.
	_, err = svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "rollback", OrgID: org(), Attachments: []string{"synthetic-missing-object"}}, actor)
	require.Error(t, err)
	var count int
	require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE type_code=$1`, code).Scan(&count))
	require.Zero(t, count)
	_, err = svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "forbidden", OrgID: org()}, 0)
	require.Error(t, err, "publication does not grant current org permission")
}

func TestD01PinnedSLAIgnoresLaterDraftChanges(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_fence_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{
		TransitionMeta: rawJSON(`{"assigned":{"sla_due_hours":2}}`),
	})
	require.NoError(t, err)
	published := publishD01(t, svc, code, actor)
	tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "pinned SLA", OrgID: org()}, actor)
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{
		TransitionMeta: rawJSON(`{"assigned":{"sla_due_hours":9}}`),
	})
	require.NoError(t, err)
	updated, err := svc.Transition(ctx, &model.TransitionTicketRequest{ID: tk.ID, To: StatusAssigned}, actor)
	require.NoError(t, err)
	require.Equal(t, published.DefinitionVersion, *updated.DefinitionVersion)
	require.Equal(t, 2*time.Hour, updated.SLADueAt.Sub(*updated.StatusChangedAt))
}

func TestD01UnknownSnapshotFormatStopsCreate(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_format_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	publishD01(t, svc, code, actor)
	_, err = testPool.Exec(ctx, `INSERT INTO ticket_type_definitions(type_code,definition_version,definition_hash,
		schema_format_version,snapshot,source_draft_revision,published_by)
		SELECT type_code,2,definition_hash,2,snapshot,source_draft_revision,published_by
		FROM ticket_type_definitions WHERE type_code=$1 AND definition_version=1`, code)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `UPDATE ticket_types SET current_published_version=2 WHERE code=$1`, code)
	require.NoError(t, err)
	_, err = svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "unsupported format", OrgID: org()}, actor)
	require.Error(t, err, "an old reader must reject a future schema format")
}

func TestD01PublishedDictionaryReferenceSurvivesDraftEdit(t *testing.T) {
	svc, actor, _ := setupTicketAdmin(t)
	ctx := context.Background()
	code := "d01_dict_" + uniqueSuffix()
	dict := repository.NewDictRepo(testPool)
	require.NoError(t, dict.CreateType(ctx, &model.DictType{Code: code, Name: code, Enabled: true}))
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	fields := []model.TicketTypeFieldInput{{FieldKey: "env", FieldLabel: "env", FieldType: "select", FieldOptions: rawJSON(`["dict:` + code + `"]`)}}
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: fields})
	require.NoError(t, err)
	publishD01(t, svc, code, actor)
	fields[0].FieldOptions = rawJSON(`["literal"]`)
	_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: fields})
	require.NoError(t, err)
	require.ErrorIs(t, dict.DeleteTypeByCode(ctx, code), repository.ErrDictTypeInUse, "draft changes must not free a published reference")
}

func TestD01PublicationAcceptsPersistedNumberAndRating(t *testing.T) {
	for _, kind := range []string{"number", "rating", "formula"} {
		t.Run(kind, func(t *testing.T) {
			svc, actor, org := setupTicketAdmin(t)
			ctx := context.Background()
			code := "d01_numeric_" + uniqueSuffix()
			_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
			require.NoError(t, err)
			fields := []model.TicketTypeFieldInput{{FieldKey: "score", FieldLabel: "score", FieldType: kind}}
			if kind == "formula" {
				fields[0].FieldType = "number"
				fields[0].FieldMeta = rawJSON(`{"formula":"base + 1"}`)
				fields = append(fields, model.TicketTypeFieldInput{FieldKey: "base", FieldLabel: "base", FieldType: "number"})
			}
			_, err = svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: fields})
			require.NoError(t, err)
			publishD01(t, svc, code, actor)
			tk, err := svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "numeric", OrgID: org(), CustomData: rawJSON(`{"base":2,"score":3}`)}, actor)
			require.NoError(t, err)
			require.NotNil(t, tk.DefinitionVersion)
			// Persisted empty options are accepted, but actual unsupported options are not.
			_, err = testPool.Exec(ctx, `UPDATE ticket_type_fields SET field_options='["invalid"]' WHERE type_code=$1 AND field_key='score'`, code)
			require.NoError(t, err)
			draft, err := svc.ticketRepo.GetTicketType(ctx, code)
			require.NoError(t, err)
			_, _, err = svc.PublishTicketType(ctx, draft, actor)
			require.Error(t, err)
		})
	}
}

package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type unavailableEvidenceDict struct{}

func (unavailableEvidenceDict) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, errors.New("synthetic unavailable dictionary")
}

func TestD02EvidenceG2RequiresDictionaryAndPreservesRetiredValues(t *testing.T) {
	svc := &Service{}
	field := &model.TicketTypeField{FieldKey: "site", FieldType: "select", FieldOptions: json.RawMessage(`["dict:test"]`)}
	before := &model.Ticket{CustomData: json.RawMessage(`{"site":"retired"}`)}
	collect := []evidenceCollectField{{Key: "site", Required: true}}
	patch := json.RawMessage(`{"site":"retired"}`)
	_, err := svc.validateEvidenceTransitionData(context.Background(), unavailableEvidenceDict{}, before, []*model.TicketTypeField{field}, collect, patch, nil)
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	field.FieldOptions = json.RawMessage(`["active"]`)
	_, err = svc.validateEvidenceTransitionData(context.Background(), nil, before, []*model.TicketTypeField{field}, collect, patch, nil)
	require.NoError(t, err)
	_, err = svc.validateEvidenceTransitionData(context.Background(), nil, before, []*model.TicketTypeField{field}, collect, json.RawMessage(`{"site":"inactive"}`), nil)
	require.Error(t, err)
}

func TestD02TransitionInputAndFingerprint(t *testing.T) {
	decode := func(t *testing.T, body, key string) (evidenceTransitionRequest, error) {
		t.Helper()
		var req model.TransitionTicketRequest
		require.NoError(t, json.Unmarshal([]byte(body), &req))
		require.Empty(t, req.IdempotencyKey, "the body cannot set the operation key")
		req.IdempotencyKey = key
		return parseEvidenceTransitionRequest(&req)
	}
	legacy, err := decode(t, `{"id":"1","to":"next","comment":"legacy"}`, "")
	require.NoError(t, err)
	require.False(t, legacy.versioned())
	for _, raw := range []string{`7`, `null`, `""`, `"0"`, `"01"`, `"+1"`, `" 1"`, `"1e1"`, `"9223372036854775808"`} {
		for _, field := range []string{"expected_revision", "definition_version"} {
			t.Run(field+"/"+raw, func(t *testing.T) {
				fields := map[string]json.RawMessage{"id": []byte(`"1"`), "to": []byte(`"next"`), "expected_revision": []byte(`"1"`), "definition_version": []byte(`"2"`)}
				fields[field] = []byte(raw)
				body, err := json.Marshal(fields)
				require.NoError(t, err)
				_, err = decode(t, string(body), "operation")
				require.Error(t, err)
			})
		}
	}
	for _, body := range []string{
		`{"id":"1","to":"next","custom_data_patch":{}}`,
		`{"id":"1","to":"next","expected_revision":"1","definition_version":"2","custom_data_patch":null}`,
		`{"id":"1","to":"next","expected_revision":"1","definition_version":"2","custom_data_patch":[]}`,
	} {
		_, err := decode(t, body, "operation")
		require.Error(t, err)
	}
	const body = `{"id":"1","to":"next","expected_revision":"9223372036854775807","definition_version":"2","custom_data_patch":{"z":9007199254740993,"a":"text"}}`
	_, err = decode(t, body, "")
	require.Error(t, err)
	a, err := decode(t, body, "operation")
	require.NoError(t, err)
	b, err := decode(t, `{"id":"1","to":"next","expected_revision":"9223372036854775807","definition_version":"2","custom_data_patch":{"a":"text", "z":9007199254740993}}`, "operation")
	require.NoError(t, err)
	require.Equal(t, a.hash, b.hash)
	c, err := decode(t, `{"id":"1","to":"next","expected_revision":"9223372036854775807","definition_version":"2","custom_data_patch":{"a":"text","z":9007199254740992}}`, "operation")
	require.NoError(t, err)
	require.NotEqual(t, a.hash, c.hash)
}

func TestD02CollectFieldsUseCandidateAndRejectUntrustedPatch(t *testing.T) {
	ctx := context.Background()
	svc := &Service{}
	before := &model.Ticket{Status: "open", CustomData: json.RawMessage(`{"switch":"hide","explanation":"earlier evidence","owner":"original","computed":123,"unknown":"legacy"}`)}
	fields := []*model.TicketTypeField{
		{FieldKey: "switch", FieldType: "input"},
		{FieldKey: "explanation", FieldType: "input", FieldMeta: json.RawMessage(`{"visible_when":"switch == 'show'"}`)},
		{FieldKey: "owner", FieldType: "input", FieldMeta: json.RawMessage(`{"write_authority":"domain","visible_when":"switch == 'hide'"}`)},
		{FieldKey: "computed", FieldType: "number", FieldMeta: json.RawMessage(`{"formula":"2 + 3"}`)},
	}
	raw := json.RawMessage(`{"open>next":{"collect_fields":[{"key":"switch","required":false},{"key":"explanation","required":true}]}}`)
	collect, err := transitionCollectFields(raw, "open", "next")
	require.NoError(t, err)
	require.Error(t, rejectLegacyEvidenceTransition(raw, "open", "next", nil))
	require.NoError(t, rejectLegacyEvidenceTransition(raw, "next", "closed", nil))
	require.Error(t, rejectLegacyEvidenceTransition(raw, "next", "closed", &TransitionMeta{CommentRequired: true}))
	_, err = transitionCollectFields(json.RawMessage(`{"open>next":{"collect_fields":null}}`), "open", "next")
	require.Error(t, err)
	_, err = transitionCollectFields(json.RawMessage(`{"open>next":{"collect_fields":[{"key":"explanation"}]}}`), "open", "next")
	require.Error(t, err)
	for _, patch := range []string{
		`{"switch":"show"}`,
		`{"explanation":"new"}`,
		`{"switch":"show","explanation":"  "}`,
		`{"switch":"show","explanation":null}`,
		`{"switch":"show","explanation":42}`,
		`{"switch":"show","explanation":"new","unknown":1}`,
		`{"switch":"show","explanation":"new","owner":"forged"}`,
		`{"switch":"show","explanation":"new","computed":20}`,
	} {
		t.Run(patch, func(t *testing.T) {
			_, err := svc.validateEvidenceTransitionData(ctx, nil, before, fields, collect, json.RawMessage(patch), nil)
			require.Error(t, err)
		})
	}
	for _, key := range []string{"owner", "computed"} {
		_, err := svc.validateEvidenceTransitionData(ctx, nil, before, fields, []evidenceCollectField{{Key: key}}, json.RawMessage(`{}`), nil)
		require.Error(t, err, "misconfigured metadata must not make a protected field writable")
	}
	after, err := svc.validateEvidenceTransitionData(ctx, nil, before, fields, collect, json.RawMessage(`{"switch":"show","explanation":"new"}`), nil)
	require.NoError(t, err)
	require.JSONEq(t, `{"switch":"show","explanation":"new","owner":"original","computed":5,"unknown":"legacy"}`, string(after))
	require.JSONEq(t, `{"switch":"hide","explanation":"earlier evidence","owner":"original","computed":123,"unknown":"legacy"}`, string(before.CustomData))
	_, err = svc.validateEvidenceTransitionData(ctx, nil, before, fields, collect, json.RawMessage(`{"switch":"show","explanation":"new"}`), nil)
	require.NoError(t, err, "the immutable field structs can be reused")
}

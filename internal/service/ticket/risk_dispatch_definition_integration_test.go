//go:build integration

package ticket

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func newRiskDispatchDefinition(t *testing.T, svc *Service, actor int64, meta string) (string, *model.TicketTypeDefinition) {
	t.Helper()
	ctx := context.Background()
	code := "risk_profile_" + uniqueSuffix()
	_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: code})
	require.NoError(t, err)
	typ, _ := riskDispatchDefinitionFixture()
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{States: typ.States, Transitions: typ.Transitions, TransitionMeta: json.RawMessage(meta)})
	require.NoError(t, err)
	return code, publishD01(t, svc, code, actor)
}

func TestRiskDispatchDefinitionUsesPublishedPinAndRejectsRepublish(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code, definition := newRiskDispatchDefinition(t, svc, actor, `{}`)
	req := &model.CreateTicketRequest{TypeCode: code, OrgID: org(), Title: "Synthetic dispatch", DefinitionVersion: strconv.FormatInt(definition.DefinitionVersion, 10), DefinitionHash: definition.DefinitionHash}
	// A draft-only incompatible edit cannot replace the published contract.
	_, err := svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{Transitions: json.RawMessage(`{"open":["assigned"],"assigned":["in_progress"],"in_progress":["pending_verify"],"pending_verify":["closed"],"closed":[]}`)})
	require.NoError(t, err)
	prepared, err := svc.PrepareRiskRemediationDispatchCreate(ctx, req, actor)
	require.NoError(t, err)
	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		locked, err := svc.LockRiskDispatchCreateTx(ctx, tx, prepared)
		require.NoError(t, err)
		require.True(t, locked.remediation)
		return nil
	}))
	changed := publishD01(t, svc, code, actor)
	require.Greater(t, changed.DefinitionVersion, definition.DefinitionVersion)
	require.ErrorIs(t, repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		_, err := svc.LockRiskDispatchCreateTx(ctx, tx, prepared)
		return err
	}), errcode.ErrConcurrentModification)
	req.DefinitionVersion, req.DefinitionHash = strconv.FormatInt(changed.DefinitionVersion, 10), changed.DefinitionHash
	_, err = svc.PrepareRiskRemediationDispatchCreate(ctx, req, actor)
	require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
	_, err = svc.PrepareRiskDispatchCreate(ctx, req, actor)
	require.NoError(t, err, "disabled fixed-profile gate retains the existing general bridge contract")
}

func TestRiskDispatchDefinitionChecksInitialActorAndValidatorBeforeInsert(t *testing.T) {
	for _, tc := range []struct {
		name, meta string
		want       error
	}{
		{"actual dispatcher", `{"open>assigned":{"roles":["creator"]}}`, errcode.ErrNoPermission},
		{"initial validator", `{"open>assigned":{"validator":"priority == 1"}}`, errcode.ErrInvalidParams},
	} {
		t.Run(tc.name, func(t *testing.T) {
			svc, actor, org := setupTicketAdmin(t)
			ctx := context.Background()
			code, definition := newRiskDispatchDefinition(t, svc, actor, tc.meta)
			var owner int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password,status) VALUES($1,'synthetic-hash',1) RETURNING id`, code).Scan(&owner))
			prepared, err := svc.PrepareRiskRemediationDispatchCreate(ctx, &model.CreateTicketRequest{TypeCode: code, OrgID: org(), Title: "Synthetic guard", Priority: 3, DefinitionVersion: strconv.FormatInt(definition.DefinitionVersion, 10), DefinitionHash: definition.DefinitionHash}, actor)
			require.NoError(t, err)
			var before, after int64
			linked := false
			err = repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
				locked, err := svc.LockRiskDispatchCreateTx(ctx, tx, prepared)
				if err != nil {
					return err
				}
				require.NoError(t, tx.QueryRow(ctx, `SELECT last_value FROM tickets_id_seq`).Scan(&before))
				_, err = svc.CreateRiskDispatchTicketTx(ctx, tx, locked, owner, func(context.Context, pgx.Tx, int64) error { linked = true; return nil })
				require.NoError(t, tx.QueryRow(ctx, `SELECT last_value FROM tickets_id_seq`).Scan(&after))
				return err
			})
			requireErrCode(t, err, tc.want.(*errcode.Error).Code)
			require.Equal(t, before, after, "even a rolled-back INSERT advances the sequence; the guard must precede it")
			require.False(t, linked)
		})
	}
}

func TestRiskDispatchDefinitionInitialValidatorUsesComputedCandidate(t *testing.T) {
	svc, actor, org := setupTicketAdmin(t)
	ctx := context.Background()
	code, _ := newRiskDispatchDefinition(t, svc, actor, `{}`)
	_, err := svc.ReplaceTicketTypeFields(ctx, code, &model.ReplaceTypeFieldsRequest{Fields: []model.TicketTypeFieldInput{
		{FieldKey: "base", FieldLabel: "Base", FieldType: "number"},
		{FieldKey: "computed", FieldLabel: "Computed", FieldType: "number", FieldMeta: json.RawMessage(`{"formula":"base * 2"}`)},
	}})
	require.NoError(t, err)
	_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{TransitionMeta: json.RawMessage(`{"open>assigned":{"validator":"custom_data[\"computed\"] == 6"}}`)})
	require.NoError(t, err)
	definition := publishD01(t, svc, code, actor)
	prepared, err := svc.PrepareRiskRemediationDispatchCreate(ctx, &model.CreateTicketRequest{TypeCode: code, OrgID: org(), Title: "Synthetic computed", CustomData: json.RawMessage(`{"base":3,"computed":999}`), DefinitionVersion: strconv.FormatInt(definition.DefinitionVersion, 10), DefinitionHash: definition.DefinitionHash}, actor)
	require.NoError(t, err)
	var ticket *model.Ticket
	require.NoError(t, repository.WithIAMCommandGuard(ctx, testPool, func(ctx context.Context, tx pgx.Tx) error {
		locked, err := svc.LockRiskDispatchCreateTx(ctx, tx, prepared)
		if err != nil {
			return err
		}
		ticket, err = svc.CreateRiskDispatchTicketTx(ctx, tx, locked, actor, func(context.Context, pgx.Tx, int64) error { return nil })
		return err
	}))
	require.Equal(t, StatusAssigned, ticket.Status)
	require.JSONEq(t, `{"base":3,"computed":6}`, string(ticket.CustomData))
}

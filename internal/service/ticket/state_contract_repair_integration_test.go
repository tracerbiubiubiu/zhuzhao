//go:build integration

package ticket

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

func TestStateContractRepair_ConfigurationAndLegacyEntry(t *testing.T) {
	ctx := context.Background()
	svc, actor, _, _, roles := setupTicket2a(t)
	roles[actor] = []string{"admin"}
	t.Run("invalid_new_graph_not_persisted", func(t *testing.T) {
		code := "reserved_new_" + uniqueSuffix()
		_, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "合成缺保留态", States: []byte(`["new","done"]`), Transitions: []byte(`{"new":["done"]}`)})
		require.Error(t, err)
		var count int
		require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_types WHERE code=$1`, code).Scan(&count))
		require.Zero(t, count)
	})
	t.Run("invalid_graph_patch_is_atomic", func(t *testing.T) {
		code := "reserved_patch_" + uniqueSuffix()
		before, err := svc.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: code, Name: "合成原合法图"})
		require.NoError(t, err)
		before, err = svc.ticketRepo.GetTicketType(ctx, code)
		require.NoError(t, err)
		_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{States: []byte(`["open","closed"]`), Transitions: []byte(`{"open":["closed"]}`)})
		require.Error(t, err)
		after, err := svc.ticketRepo.GetTicketType(ctx, code)
		require.NoError(t, err)
		require.Equal(t, before.Version, after.Version)
		require.JSONEq(t, string(before.States), string(after.States))
	})
	for _, operation := range []string{"legacy_create", "enable_legacy"} {
		t.Run(operation, func(t *testing.T) {
			code := "reserved_legacy_" + uniqueSuffix()
			_, err := testPool.Exec(ctx, `INSERT INTO ticket_types(code,name,states,transitions,is_active) VALUES($1,'合成历史坏图','["new","done"]','{"new":["done"]}',true)`, code)
			require.NoError(t, err)
			if operation == "legacy_create" {
				_, err = svc.Create(ctx, &model.CreateTicketRequest{TypeCode: code, Title: "不得创建幽灵open态", OrgID: childOrgID(t, "legacy_"+uniqueSuffix())}, actor)
				require.Error(t, err)
				var count int
				require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM tickets WHERE type_code=$1`, code).Scan(&count))
				require.Zero(t, count)
			} else {
				_, err = testPool.Exec(ctx, `UPDATE ticket_types SET is_active=false WHERE code=$1`, code)
				require.NoError(t, err)
				enabled := true
				_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{IsActive: &enabled})
				require.Error(t, err)
				var active bool
				require.NoError(t, testPool.QueryRow(ctx, `SELECT is_active FROM ticket_types WHERE code=$1`, code).Scan(&active))
				require.False(t, active)
			}
			// Historical definitions can be repaired in place without rewriting
			// old tickets, creating a second type or discarding their history.
			_, err = svc.UpdateTicketType(ctx, code, &model.UpdateTicketTypeRequest{States: []byte(defaultStates), Transitions: []byte(defaultTransitions)})
			require.NoError(t, err)
		})
	}
}

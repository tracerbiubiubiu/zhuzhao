package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type riskBindingRow func(...any) error

func (f riskBindingRow) Scan(d ...any) error { return f(d...) }

type riskBindingTx struct {
	pgx.Tx
	t       *testing.T
	tables  [2]bool
	raw     string
	failure error
	reads   int
}

func (tx *riskBindingTx) QueryRow(_ context.Context, q string, args ...any) pgx.Row {
	require.NotContains(tx.t, strings.ToUpper(q), "FOR UPDATE")
	require.NotContains(tx.t, strings.ToUpper(q), "FOR SHARE")
	tx.reads++
	return riskBindingRow(func(d ...any) error {
		if tx.failure != nil {
			return tx.failure
		}
		if strings.Contains(q, "to_regclass") {
			*d[0].(*bool) = tx.tables[0]
			*d[1].(*bool) = tx.tables[1]
			return nil
		}
		require.Equal(tx.t, int64(41), args[0])
		*d[0].(*json.RawMessage) = json.RawMessage(tx.raw)
		return nil
	})
}
func TestReadActiveRiskTicketBinding(t *testing.T) {
	repo := NewTicketRepo(nil)
	for _, tc := range []struct {
		name        string
		tables      [2]bool
		raw         string
		want        int
		unavailable bool
	}{
		{name: "uninstalled module"},
		{name: "partial links", tables: [2]bool{true, false}, unavailable: true},
		{name: "partial groups", tables: [2]bool{false, true}, unavailable: true},
		{name: "ordinary", tables: [2]bool{true, true}, raw: `[]`},
		{name: "linked", tables: [2]bool{true, true}, raw: `[{"link_id":3,"risk_id":7,"group_id":5,"revision":2,"owner_at_link":22,"owner_user_id":22,"target_org_id":4,"type_code":"incident","definition_version":1,"group_active":true}]`, want: 1},
		{name: "corrupt binding", tables: [2]bool{true, true}, raw: `null`, unavailable: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := &riskBindingTx{t: t, tables: tc.tables, raw: tc.raw}
			got, err := repo.ReadActiveRiskTicketBindingTx(context.Background(), tx, 41)
			if tc.unavailable {
				require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
			} else {
				require.NoError(t, err)
				require.Len(t, got, tc.want)
			}
			if !tc.tables[0] || !tc.tables[1] {
				require.Equal(t, 1, tx.reads)
			}
		})
	}
	failure := errors.New("synthetic database failure")
	_, err := repo.ReadActiveRiskTicketBindingTx(context.Background(), &riskBindingTx{t: t, failure: failure}, 41)
	require.ErrorIs(t, err, failure)
}

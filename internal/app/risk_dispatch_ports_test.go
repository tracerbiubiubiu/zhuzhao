package app

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type dispatchDefinitionFaultReader struct{ err error }

func (r dispatchDefinitionFaultReader) QueryRow(context.Context, string, ...any) pgx.Row {
	return dispatchDefinitionFaultRow{r.err}
}
func (r dispatchDefinitionFaultReader) Query(context.Context, string, ...any) (pgx.Rows, error) {
	return nil, r.err
}

type dispatchDefinitionFaultRow struct{ err error }

func (r dispatchDefinitionFaultRow) Scan(...any) error { return r.err }

func TestDispatchDefinitionUsesOwningReaderFailureBoundary(t *testing.T) {
	fault := errors.New("synthetic owning transaction read fault")
	var err error
	require.NotPanics(t, func() {
		_, err = (riskDispatchTicketPorts{}).CurrentPublished(context.Background(), dispatchDefinitionFaultReader{fault}, "synthetic_risk")
	})
	require.ErrorIs(t, err, fault)
}

type dispatchDefinitionRowFunc func(...any) error

func (f dispatchDefinitionRowFunc) Scan(out ...any) error { return f(out...) }

type dispatchDefinitionReader struct {
	dispatchDefinitionFaultReader
	row pgx.Row
}

func (r dispatchDefinitionReader) QueryRow(context.Context, string, ...any) pgx.Row { return r.row }

func TestDispatchDefinitionPreviewRejectsIncompatibleGateOnPublication(t *testing.T) {
	for _, tc := range []struct {
		name             string
		gate, compatible bool
	}{
		{"gate on incompatible", true, false},
		{"gate on compatible", true, true},
		{"existing ordinary publication", false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			graph := `{"open":["assigned"],"assigned":["in_progress"],"in_progress":["closed"],"closed":[]}`
			states := `["open","assigned","in_progress","closed"]`
			if tc.compatible {
				states = `["open","assigned","in_progress","pending_verify","closed"]`
				graph = `{"open":["assigned"],"assigned":["in_progress"],"in_progress":["pending_verify"],"pending_verify":["pending_verify","in_progress","closed"],"closed":[]}`
			}
			raw, err := json.Marshal(model.TicketDefinitionSnapshot{
				Type:   model.TicketType{Code: "synthetic_risk", States: json.RawMessage(states), Transitions: json.RawMessage(graph), TransitionMeta: json.RawMessage(`{}`)},
				Fields: []*model.TicketTypeField{},
			})
			require.NoError(t, err)
			reader := dispatchDefinitionReader{row: dispatchDefinitionRowFunc(func(out ...any) error {
				version, hash, format := int64(7), strings.Repeat("a", 64), 1
				*out[0].(**int64) = &version
				*out[1].(**string) = &hash
				*out[2].(**int) = &format
				*out[3].(*json.RawMessage) = raw
				return nil
			})}
			ref, err := (riskDispatchTicketPorts{remediation: tc.gate}).CurrentPublished(context.Background(), reader, "synthetic_risk")
			if tc.gate && !tc.compatible {
				require.ErrorIs(t, err, errcode.ErrServiceUnavailable)
				require.Zero(t, ref.Version)
			} else {
				require.NoError(t, err)
				require.Equal(t, int64(7), ref.Version)
			}
		})
	}
}

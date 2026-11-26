package riskremediation

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/require"
)

func TestRequiresCallerGuardAndAdapters(t *testing.T) {
	s := New(nil, Callbacks{})
	_, err := s.FinishTx(context.Background(), nil, FinishCommand{})
	require.Error(t, err)
}

// Every adapter slot is required. A partially wired service must refuse to run
// rather than silently skipping a missing defense contract.
func TestRequiresAllAcceptanceAdapters(t *testing.T) {
	stub := Callbacks{
		Admit:               func(context.Context, pgx.Tx, Access) error { return nil },
		Prelock:             func(context.Context, pgx.Tx, Access) error { return nil },
		Authorize:           func(context.Context, pgx.Tx, Access) error { return nil },
		TicketPolicy:        func(context.Context, pgx.Tx, TicketCheck) (Policy, error) { return Policy{}, nil },
		Evidence:            func(context.Context, pgx.Tx, EvidenceCheck) error { return nil },
		ApplyTicket:         func(context.Context, pgx.Tx, TicketEffect) (int64, error) { return 0, nil },
		IndependentApprover: func(context.Context, pgx.Tx, Access) error { return nil },
	}
	nilOut := func(name string) Callbacks {
		partial := stub
		switch name {
		case "Admit":
			partial.Admit = nil
		case "Prelock":
			partial.Prelock = nil
		case "Authorize":
			partial.Authorize = nil
		case "TicketPolicy":
			partial.TicketPolicy = nil
		case "Evidence":
			partial.Evidence = nil
		case "ApplyTicket":
			partial.ApplyTicket = nil
		case "IndependentApprover":
			partial.IndependentApprover = nil
		}
		return partial
	}
	for _, name := range []string{"Admit", "Prelock", "Authorize", "TicketPolicy", "Evidence", "ApplyTicket", "IndependentApprover"} {
		t.Run(name, func(t *testing.T) {
			s := New(nil, nilOut(name))
			_, err := s.RequestAcceptanceTx(context.Background(), nil, AcceptanceRequest{})
			require.Error(t, err)
			_, err = s.ReworkAcceptanceTx(context.Background(), nil, AcceptanceRework{})
			require.Error(t, err)
		})
	}
}

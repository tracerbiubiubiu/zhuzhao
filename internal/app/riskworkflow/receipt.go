// Package riskworkflow is an isolated in-process composition candidate for
// owning risk/ticket ports. Production composition belongs at internal/app;
// this package's PG tests do not establish production routing or assembly.
package riskworkflow

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// ReceiptService is safe to assemble with the existing ticket service and its
// SAME fact pool. It does not expose the remediation kernel or write callbacks.
// The app/handler owner may use Receipt from the existing risk operation route;
// this package neither registers that route nor changes its response contract.
type ReceiptService struct {
	pool    *pgxpool.Pool
	tickets *ticket.Service
}

func NewReceiptService(pool *pgxpool.Pool, tickets *ticket.Service) *ReceiptService {
	return &ReceiptService{pool: pool, tickets: tickets}
}

// Receipt recovers only this authenticated actor's original operation. C04
// serializes current credentials, L1, original ticket read and EVERY original
// risk member with IAM changes. Detached members remain protected by ReceiptTx.
// No transition edge is re-consumed and no business/receipt/intent is written.
func (s *ReceiptService) Receipt(ctx context.Context, actor int64, operation string) (riskremediation.Result, error) {
	if s == nil || s.pool == nil || s.tickets == nil {
		return riskremediation.Result{}, errcode.ErrServiceUnavailable
	}
	if actor <= 0 || strings.TrimSpace(operation) == "" || len(operation) > 128 {
		return riskremediation.Result{}, errcode.ErrInvalidParams
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	path := "/api/v1/risk-items/remediation-operations/" + operation
	if !ok || purpose.UserID != actor || purpose.Path != path || purpose.Method != "GET" || purpose.SelfService {
		return riskremediation.Result{}, errcode.ErrNoPermission
	}
	var result riskremediation.Result
	err := repository.WithIAMCommandGuard(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		return s.tickets.WithRiskWorkflowAuthorizationTx(ctx, tx, actor, []ticket.RiskWorkflowRoute{{Path: path, Method: "GET"}}, func(ctx context.Context) error {
			callbacks := s.receiptCallbacks()
			var err error
			result, err = riskremediation.New(s.pool, callbacks).ReceiptTx(ctx, tx, actor, operation)
			return err
		})
	})
	if err != nil {
		return riskremediation.Result{}, err
	}
	return result, nil
}

func (s *ReceiptService) receiptCallbacks() riskremediation.Callbacks {
	exact := func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
		if a.Action != "receipt" || !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, s.pool) != tx {
			return errcode.ErrNoPermission
		}
		return nil
	}
	return riskremediation.Callbacks{
		Admit: exact,
		// Receipt does not evaluate or change a definition/organization. Its ticket
		// access is checked before group/risk/ticket/link locks, and again on every
		// original member. No earlier domain lock is acquired after the kernel locks.
		Prelock: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
			if err := exact(ctx, tx, a); err != nil {
				return err
			}
			if a.TicketID > 0 {
				return s.tickets.CheckRiskWorkflowReadTx(ctx, tx, a.ActorID, a.TicketID)
			}
			return nil
		},
		Authorize: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
			if err := exact(ctx, tx, a); err != nil {
				return err
			}
			if a.TicketID > 0 {
				if err := s.tickets.CheckRiskWorkflowReadTx(ctx, tx, a.ActorID, a.TicketID); err != nil {
					return err
				}
			}
			// The kernel first checks the original ticket alone, then enumerates
			// every original risk member. This is a read check, not a grant for
			// any member or a write action.
			if a.RiskID <= 0 {
				if a.TicketID > 0 {
					return nil
				}
				return errcode.ErrNotFound
			}
			_, _, _, err := repository.GetRiskItemVisible(ctx, tx, a.RiskID, a.ActorID)
			if errors.Is(err, repository.ErrRiskNotFound) {
				return errcode.ErrNotFound
			}
			if err != nil {
				return errcode.ErrServiceUnavailable
			}
			return nil
		},
		// Required kernel callbacks stay closed: this service cannot accidentally be
		// repurposed into a write adapter with permissive placeholder authorization.
		TicketPolicy: func(context.Context, pgx.Tx, riskremediation.TicketCheck) (riskremediation.Policy, error) {
			return riskremediation.Policy{}, errcode.ErrServiceUnavailable
		},
		Evidence: func(context.Context, pgx.Tx, riskremediation.EvidenceCheck) error {
			return errcode.ErrServiceUnavailable
		},
		ApplyTicket: func(context.Context, pgx.Tx, riskremediation.TicketEffect) (int64, error) {
			return 0, errcode.ErrServiceUnavailable
		},
	}
}

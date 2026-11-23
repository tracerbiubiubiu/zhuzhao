package riskworkflow

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// ReworkAcceptance returns one expired accepted item on an active linked
// ticket to remediation, using the current guarded route and pinned ticket.
func (s *Service) ReworkAcceptance(ctx context.Context, c riskremediation.AcceptanceRework, ticketInput ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	if c.TicketBinding != "" || c.TicketID <= 0 || c.ExpectedTicketRevision <= 0 {
		return riskremediation.Result{}, errcode.ErrInvalidParams
	}
	input, binding, err := ticket.PrepareRiskWorkflowTicketInput(ticketInput)
	if err != nil {
		return riskremediation.Result{}, err
	}
	c.TicketBinding = binding
	return s.acceptanceCommand(ctx, c.ActorID, acceptReworkPath, input, func(ctx context.Context, tx pgx.Tx, callbacks riskremediation.Callbacks) (riskremediation.Result, error) {
		prelock := callbacks.Prelock
		callbacks.Prelock = func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
			if err := prelock(ctx, tx, a); err != nil || a.Action != "accept_rework" {
				return err
			}
			// Bind the submitted approval and link to the authorized risk and
			// ticket before the kernel locks the approval. Exact receipt replay
			// has its own historical authorization and bypasses this live check.
			bound, err := repository.TxBool(ctx, tx, `
				SELECT EXISTS (
					SELECT 1 FROM risk_acceptances a
					JOIN risk_ticket_links l ON l.id = a.link_id AND l.risk_id = a.risk_id
					JOIN risk_dispatch_groups g ON g.id = l.group_id
					WHERE a.id = $1 AND a.risk_id = $2 AND l.id = $3 AND g.ticket_id = $4
					  AND l.ended_at IS NULL AND g.ended_at IS NULL
				)`, c.AcceptanceID, a.RiskID, c.LinkID, a.TicketID)
			if err != nil {
				return errcode.ErrServiceUnavailable
			}
			if !bound {
				return errcode.ErrConflict
			}
			return nil
		}
		return riskremediation.New(s.pool, callbacks).ReworkAcceptanceTx(ctx, tx, c)
	})
}

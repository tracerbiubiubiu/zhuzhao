package riskworkflow

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

func requireAcceptanceManagement(ctx context.Context, tx pgx.Tx, riskID, actor int64) error {
	manage, err := repository.CanManageRiskItem(ctx, tx, riskID, actor)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	if !manage {
		return errcode.ErrNoPermission
	}
	return nil
}

// A primary-scope membership prefilters the candidates. The existing risk
// predicate still checks live management, and ticket owns live IAM and edges.
func (s *Service) requireIndependentAcceptanceApprover(ctx context.Context, tx pgx.Tx, scope *ticket.RiskWorkflowScope, a riskremediation.Access) error {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT u.id
		FROM risk_items r
		JOIN organizations primary_org ON primary_org.id = r.org_id
		JOIN organizations manager_org ON primary_org.path <@ manager_org.path
		JOIN user_orgs m ON m.org_id = manager_org.id
		JOIN users u ON u.id = m.user_id
		WHERE r.id = $1 AND u.id <> $2 AND u.status = 1
		  AND m.org_member_role IN ('admin', 'owner')
		ORDER BY u.id`, a.RiskID, a.ActorID)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowTo[int64])
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	for _, candidate := range candidates {
		err = requireAcceptanceManagement(ctx, tx, a.RiskID, candidate)
		if errors.Is(err, errcode.ErrNoPermission) {
			continue
		}
		if err != nil {
			return err
		}
		err = s.tickets.CheckRiskAcceptanceApproverTx(ctx, tx, scope, candidate)
		if errors.Is(err, errcode.ErrNoPermission) {
			continue
		}
		return err
	}
	return errcode.ErrNoPermission
}

// These command adapters do not implement acceptance requests. Keep the
// required callback closed even if a new kernel call reaches it.
func denyIndependentAcceptanceApprover(context.Context, pgx.Tx, riskremediation.Access) error {
	return errcode.ErrServiceUnavailable
}

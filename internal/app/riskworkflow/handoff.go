package riskworkflow

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskdispatch"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

func (s *Service) Handoff(ctx context.Context, c riskremediation.HandoffCommand) (riskremediation.Result, error) {
	if s == nil || s.pool == nil || s.tickets == nil {
		return riskremediation.Result{}, errcode.ErrServiceUnavailable
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	path := ticket.RiskRetestHandoffPath
	if !ok || purpose.UserID != c.ActorID || purpose.Path != path || purpose.Method != "POST" || purpose.SelfService {
		return riskremediation.Result{}, errcode.ErrNoPermission
	}
	var result riskremediation.Result
	err := repository.WithIAMCommandGuard(ctx, s.pool, func(ctx context.Context, outer pgx.Tx) error {
		var current context.Context
		var pin *ticket.RiskWorkflowScope
		var observedOwner int64
		observedOrgs := map[int64]bool{}
		var observedFinding string
		receipt := NewReceiptService(s.pool, s.tickets).receiptCallbacks()
		callbacks := riskremediation.Callbacks{
			Admit: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
				return s.tickets.WithRiskWorkflowAuthorizationTx(ctx, tx, c.ActorID, []ticket.RiskWorkflowRoute{{Path: path, Method: "POST"}}, func(fresh context.Context) error {
					current = fresh
					return nil
				})
			},
			Prelock: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
				if a.Action == "receipt" {
					return receipt.Prelock(current, tx, a)
				}
				if err := s.tickets.CheckRiskWorkflowReadTx(current, tx, c.ActorID, c.TicketID); err != nil {
					return err
				}
				members, err := assignmentMembers(current, tx, c.TicketID)
				if err != nil {
					return err
				}
				if len(members) == 0 {
					return errcode.ErrNotFound
				}
				orgs := memberOrgs(members)
				for _, org := range orgs {
					observedOrgs[org] = true
				}
				pin, err = s.tickets.PrelockRiskRetestHandoffTx(current, tx, c.TicketID, c.ActorID, orgs...)
				if err != nil {
					return err
				}
				err = tx.QueryRow(current, `SELECT finding_key,owner_user_id FROM risk_dispatch_groups WHERE ticket_id=$1 AND ended_at IS NULL`, c.TicketID).Scan(&observedFinding, &observedOwner)
				if errors.Is(err, pgx.ErrNoRows) {
					return errcode.ErrNotFound
				}
				if err != nil {
					return err
				}
				return riskdispatch.LockHandoffGroupIdentities(current, tx, observedFinding, observedOwner, c.SuccessorUserID)
			},
			Authorize: func(ctx context.Context, tx pgx.Tx, a riskremediation.Access) error {
				if a.Action == "receipt" {
					return receipt.Authorize(current, tx, a)
				}
				if a.RiskID == 0 {
					return s.tickets.CheckRiskWorkflowReadTx(current, tx, c.ActorID, c.TicketID)
				}
				if !observedOrgs[a.OrgID] {
					return errcode.ErrConflict
				}
				allowed, err := repository.CanManageRiskItem(current, tx, a.RiskID, c.ActorID)
				if err != nil {
					return err
				}
				if !allowed {
					return errcode.ErrNoPermission
				}
				return nil
			},
		}
		check := func(ctx context.Context, tx pgx.Tx, group riskremediation.HandoffScope) (string, error) {
			if pin == nil || group.OwnerID != observedOwner || group.FindingKey != observedFinding {
				return "", errcode.ErrConflict
			}
			members, err := assignmentMembers(current, tx, c.TicketID)
			if err != nil {
				return "", err
			}
			for _, org := range memberOrgs(members) {
				if !observedOrgs[org] {
					return "", errcode.ErrConflict
				}
			}
			if err := s.tickets.CheckRiskRetestSuccessorTx(current, tx, pin, c.SuccessorUserID); err != nil {
				return "", err
			}
			visible, err := repository.CanReadActiveTicketRisks(current, tx, c.TicketID, c.SuccessorUserID)
			if err != nil {
				return "", err
			}
			if !visible {
				return "", errcode.ErrNoPermission
			}
			return riskdispatch.HandoffGroupKey(group.FindingKey, c.SuccessorUserID, group.ManagementOrgID, group.ApprovedScope, riskdispatch.Target{
				OrganizationID: group.TargetOrgID, TypeCode: group.TypeCode, DefinitionVersion: c.DefinitionVersion, DefinitionHash: pin.DefinitionHash(),
			}), nil
		}
		var err error
		result, err = riskremediation.New(s.pool, callbacks).HandoffTx(ctx, outer, c, check)
		return err
	})
	if err != nil {
		return riskremediation.Result{}, err
	}
	return result, nil
}

package riskworkflow

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

const contextPrefix = "/api/v1/risk-items/remediation-context/"

var workflowContextActions = []struct {
	name, route, ticketRoute string
}{
	{"finish", "/api/v1/risk-items/remediation-finish", "/api/v1/tickets/transition"},
	{"review", "/api/v1/risk-items/remediation-review", "/api/v1/tickets/transition"},
	{"accept_request", acceptRequestPath, ""},
	{"accept_decision", acceptDecisionPath, ""},
	{"accept_rework", "/api/v1/risk-items/remediation-accept-rework", "/api/v1/tickets/transition"},
	{"assign_handler", assignHandlerPath, "/api/v1/tickets/assign"},
	{"accept_assignment", acceptAssignmentPath, "/api/v1/tickets/transition"},
	{"handoff", ticket.RiskRetestHandoffPath, ""},
}

// Current reads actionable versions under the same IAM barrier as original
// receipt queries. A ticket permission alone cannot expose linked risk facts.
func (s *ReceiptService) Current(ctx context.Context, actorID, ticketID int64) (model.RiskWorkflowContext, error) {
	if s == nil || s.pool == nil || s.tickets == nil {
		return model.RiskWorkflowContext{}, errcode.ErrServiceUnavailable
	}
	if actorID <= 0 || ticketID <= 0 {
		return model.RiskWorkflowContext{}, errcode.ErrInvalidParams
	}
	path := contextPrefix + strconv.FormatInt(ticketID, 10)
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actorID || purpose.Path != path || purpose.Method != "GET" || purpose.SelfService {
		return model.RiskWorkflowContext{}, errcode.ErrNoPermission
	}
	var result model.RiskWorkflowContext
	err := repository.WithIAMCommandGuard(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); err != nil {
			return err
		}
		return s.tickets.WithRiskWorkflowAuthorizationTx(ctx, tx, actorID, []ticket.RiskWorkflowRoute{{Path: path, Method: "GET"}}, func(ctx context.Context) error {
			if err := s.tickets.CheckRiskWorkflowReadTx(ctx, tx, actorID, ticketID); err != nil {
				return err
			}
			result.TicketID = ticketID
			result.Items = []model.RiskWorkflowItem{}
			var pin *int64
			var groupID int64
			err := tx.QueryRow(ctx, `
				SELECT t.revision, t.definition_version, t.status, g.id
				FROM tickets t JOIN risk_dispatch_groups g ON g.ticket_id = t.id
				WHERE t.id = $1
			`, ticketID).Scan(&result.TicketRevision, &pin, &result.Status, &groupID)
			if errors.Is(err, pgx.ErrNoRows) {
				return errcode.ErrNotFound
			}
			if err != nil {
				return err
			}
			if pin != nil {
				result.DefinitionVersion = *pin
			}
			rows, err := tx.Query(ctx, `
				SELECT id, risk_id, revision, proposal_version, remediation_state
				FROM risk_ticket_links
				WHERE group_id = $1 AND (ended_at IS NULL OR ($2 = 'closed' AND end_reason = 'ticket_closed'))
				ORDER BY id
			`, groupID, result.Status)
			if err != nil {
				return err
			}
			result.Items, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.RiskWorkflowItem, error) {
				var item model.RiskWorkflowItem
				err := row.Scan(&item.LinkID, &item.RiskID, &item.LinkRevision, &item.ProposalVersion, &item.RemediationState)
				return item, err
			})
			if err != nil {
				return err
			}
			for i := range result.Items {
				item := &result.Items[i]
				risk, _, _, err := repository.GetRiskItemVisible(ctx, tx, item.RiskID, actorID)
				if errors.Is(err, repository.ErrRiskNotFound) {
					return errcode.ErrNotFound
				}
				if err != nil {
					return err
				}
				item.Title, item.RiskRevision = risk.Title, risk.Revision
				item.Submissions, err = readWorkflowSubmissions(ctx, tx, item.LinkID)
				if err != nil {
					return err
				}
				manage, err := repository.CanManageRiskItem(ctx, tx, item.RiskID, actorID)
				if err != nil {
					return err
				}
				item.IsRiskOwner = risk.OwnerUserID != nil && *risk.OwnerUserID == actorID
				item.CanManageRisk = manage
				if item.RemediationState == "pending_acceptance" {
					var acceptanceID, acceptanceRevision int64
					err = tx.QueryRow(ctx, `
						SELECT id, revision, valid_until <= clock_timestamp()
							OR owner_at_request IS DISTINCT FROM $3::bigint
							OR risk_revision <> $4
							OR link_revision IS DISTINCT FROM $5::bigint
						FROM risk_acceptances
						WHERE risk_id = $1 AND link_id = $2 AND status = 'pending'
					`, item.RiskID, item.LinkID, risk.OwnerUserID, item.RiskRevision, item.LinkRevision).
						Scan(&acceptanceID, &acceptanceRevision, &item.AcceptanceRecoverable)
					if err != nil && !errors.Is(err, pgx.ErrNoRows) {
						return err
					}
					if !item.AcceptanceRecoverable && (item.CanManageRisk || item.IsRiskOwner) {
						item.AcceptanceID, item.AcceptanceRevision = acceptanceID, acceptanceRevision
					}
				}
				if item.RemediationState == "resolved" && risk.Status == "accepted" && manage &&
					(result.Status == "pending_verify" || result.Status == "in_progress") {
					err = tx.QueryRow(ctx, `
						SELECT a.id, a.revision
						FROM risk_acceptances a
						JOIN risk_items r ON r.id = a.risk_id
						WHERE a.risk_id = $1 AND a.link_id = $2 AND a.status = 'approved'
						  AND a.risk_revision = r.revision AND a.link_revision = $3
						  AND a.owner_at_request = r.owner_user_id AND r.accepted_until <= statement_timestamp()
						ORDER BY a.id DESC LIMIT 1
					`, item.RiskID, item.LinkID, item.LinkRevision).Scan(&item.ExpiredAcceptanceID, &item.ExpiredAcceptanceRevision)
					if err != nil && !errors.Is(err, pgx.ErrNoRows) {
						return err
					}
				}
			}
			result.AllowedActions = []string{}
			routes := make([]ticket.RiskWorkflowRoute, 0, len(workflowContextActions)*2)
			for _, action := range workflowContextActions {
				routes = append(routes, ticket.RiskWorkflowRoute{Path: action.route, Method: "POST"})
				if action.ticketRoute != "" {
					routes = append(routes, ticket.RiskWorkflowRoute{Path: action.ticketRoute, Method: "POST"})
				}
			}
			grants, err := s.tickets.RiskWorkflowRouteGrantsTx(ctx, tx, actorID, routes)
			if err != nil {
				return err
			}
			var cursor int
			for _, action := range workflowContextActions {
				allowed := grants[cursor]
				cursor++
				if action.ticketRoute != "" {
					allowed = allowed && grants[cursor]
					cursor++
				}
				if action.name == "handoff" {
					allowed = allowed && canHandoffContext(result)
				}
				if allowed {
					result.AllowedActions = append(result.AllowedActions, action.name)
				}
			}
			return nil
		})
	})
	if err != nil {
		return model.RiskWorkflowContext{}, err
	}
	return result, nil
}

func canHandoffContext(current model.RiskWorkflowContext) bool {
	if current.Status != "in_progress" && current.Status != "pending_verify" {
		return false
	}
	var pending bool
	for _, item := range current.Items {
		if item.RemediationState != "resolved" {
			pending = true
			if !item.CanManageRisk || (item.RemediationState == "pending_acceptance" && item.ProposalVersion == 0) {
				return false
			}
		}
	}
	return pending
}

func readWorkflowSubmissions(ctx context.Context, tx pgx.Tx, linkID int64) ([]model.RiskWorkflowSubmission, error) {
	rows, err := tx.Query(ctx, `
		SELECT s.proposal_version, s.measures, s.evidence_refs, s.created_at, r.decision, r.note, r.created_at
		FROM risk_remediation_submissions s
		LEFT JOIN risk_remediation_reviews r ON r.submission_id=s.id
		WHERE s.link_id=$1 ORDER BY s.proposal_version DESC LIMIT 5`, linkID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.RiskWorkflowSubmission, error) {
		var entry model.RiskWorkflowSubmission
		err := row.Scan(&entry.Version, &entry.Measures, &entry.Evidence, &entry.SubmittedAt, &entry.Decision, &entry.ReviewNote, &entry.ReviewedAt)
		return entry, err
	})
}

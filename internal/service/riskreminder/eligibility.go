package riskreminder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// ErrPolicyUnavailable keeps an intent recoverable when no authorized policy
// publication source is connected. Ticket node SLA is not a substitute source.
var ErrPolicyUnavailable = errors.New("first-acceptance reminder policy unavailable")

// AcceptancePolicy is an explicitly published policy bound to one immutable
// ticket definition. AcceptWithin is independent of the ticket's business SLA.
type AcceptancePolicy struct {
	DefinitionRef model.TicketDefinitionRef
	RuleID        string
	RuleVersion   int64
	AwaitingState string
	AcceptWithin  time.Duration
}

// PolicyResolver reads the authorized publication on the supplied transaction.
// It must acquire no locks, use no other connection and perform no external I/O.
// A missing resolver or publication leaves delivery failed and retryable.
type PolicyResolver func(context.Context, pgx.Tx, int64) (*AcceptancePolicy, error)

// NewEligibilityChecker checks current account, assignment, published definition
// and L1 route grants inside the delivery's IAM barrier. Being the current
// assignee supplies the existing ticket L2/L3 ownership fact; CC membership and
// historical risk/group ownership never grant reminder eligibility.
// The caller has already locked its ticket and intent; this checker only reads.
func NewEligibilityChecker(db *pgxpool.Pool, modelText string, policy PolicyResolver) EligibilityChecker {
	return func(ctx context.Context, tx pgx.Tx, ticketID, recipientID int64) (bool, string, error) {
		if db == nil || tx == nil || !repository.IAMGuardActive(ctx) || repository.IAMDB(ctx, db) != tx {
			return false, "", errcode.ErrNoPermission
		}
		if ticketID <= 0 || recipientID <= 0 {
			return false, "", errcode.ErrInvalidParams
		}
		if policy == nil {
			return false, "", ErrPolicyUnavailable
		}
		published, err := policy(ctx, tx, ticketID)
		if err != nil {
			return false, "", err
		}
		if published == nil || published.RuleID == "" || published.RuleVersion <= 0 || published.AwaitingState == "" || published.AcceptWithin <= 0 {
			return false, "", ErrPolicyUnavailable
		}

		var enabled bool
		err = tx.QueryRow(ctx, `SELECT status=1 AND deleted_at IS NULL FROM users WHERE id=$1`, recipientID).Scan(&enabled)
		if errors.Is(err, pgx.ErrNoRows) || err == nil && !enabled {
			return false, "recipient_disabled", nil
		}
		if err != nil {
			return false, "", fmt.Errorf("read reminder recipient: %w", err)
		}
		var assigned *int64
		var state, typeCode string
		var version *int64
		err = tx.QueryRow(ctx, `SELECT assigned_to,status,type_code,definition_version FROM tickets WHERE id=$1`, ticketID).Scan(&assigned, &state, &typeCode, &version)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "ticket_gone", nil
		}
		if err != nil {
			return false, "", fmt.Errorf("read reminder ticket: %w", err)
		}
		if assigned == nil || *assigned != recipientID {
			return false, "assignment_changed", nil
		}
		snapshot, ref, err := repository.NewTicketRepo(db).BoundTicketDefinitionTx(ctx, tx, typeCode, version)
		if err != nil {
			return false, "", err
		}
		if ref == nil || *ref != published.DefinitionRef {
			return false, "", ErrPolicyUnavailable
		}
		var states []string
		if json.Unmarshal(snapshot.Type.States, &states) != nil || !slices.Contains(states, published.AwaitingState) {
			return false, "", ErrPolicyUnavailable
		}
		var ruleID string
		var ruleVersion, assignmentRecipient int64
		var dispatchedAt, acceptDueAt time.Time
		var acceptedAt *time.Time
		err = tx.QueryRow(ctx, `SELECT rule_id,rule_version,assignee_id,dispatched_at,accept_due_at,accepted_at
			FROM riskreminder_assignments WHERE ticket_id=$1 ORDER BY generation DESC LIMIT 1`, ticketID).
			Scan(&ruleID, &ruleVersion, &assignmentRecipient, &dispatchedAt, &acceptDueAt, &acceptedAt)
		if errors.Is(err, pgx.ErrNoRows) {
			return false, "", ErrPolicyUnavailable
		}
		if err != nil {
			return false, "", fmt.Errorf("read reminder assignment publication: %w", err)
		}
		if assignmentRecipient != recipientID {
			return false, "assignment_changed", nil
		}
		if ruleID != published.RuleID || ruleVersion != published.RuleVersion || !acceptDueAt.Equal(dispatchedAt.Add(published.AcceptWithin)) {
			return false, "", ErrPolicyUnavailable
		}
		// Interpret lifecycle only after proving this is the dispatch-time policy.
		// A newer policy may name another awaiting state; it cannot permanently
		// suppress an intent recorded against the ticket's original publication.
		if acceptedAt != nil || state != published.AwaitingState {
			return false, "not_awaiting_acceptance", nil
		}
		visible, err := repository.CanReadActiveTicketRisks(ctx, tx, ticketID, recipientID)
		if err != nil {
			return false, "", fmt.Errorf("read reminder risk scope: %w", err)
		}
		if !visible {
			return false, "recipient_unauthorized", nil
		}

		roles, policies, err := repository.NewUserRepo(db).CurrentIAMAuthorization(ctx, recipientID)
		if err != nil {
			return false, "", fmt.Errorf("read reminder authorization: %w", err)
		}
		if len(roles) == 0 {
			return false, "recipient_unauthorized", nil
		}
		m, err := casbinmodel.NewModelFromString(modelText)
		if err != nil {
			return false, "", fmt.Errorf("parse reminder authorization model: %w", err)
		}
		enforcer, err := casbin.NewEnforcer(m)
		if err != nil {
			return false, "", fmt.Errorf("create reminder authorization evaluator: %w", err)
		}
		if _, err := enforcer.AddPolicies(policies); err != nil {
			return false, "", fmt.Errorf("load reminder authorization: %w", err)
		}
		for _, route := range [][2]string{
			{"/api/v1/tickets", "GET"}, {"/api/v1/tickets/" + strconv.FormatInt(ticketID, 10), "GET"},
			{"/api/v1/tickets/transition", "POST"}, {"/api/v1/risk-items/remediation-accept-assignment", "POST"},
		} {
			allowed := false
			for _, role := range roles {
				granted, err := enforcer.Enforce("role::"+role, route[0], route[1])
				if err != nil {
					return false, "", fmt.Errorf("evaluate reminder authorization: %w", err)
				}
				if granted {
					allowed = true
					break
				}
			}
			if !allowed {
				return false, "recipient_unauthorized", nil
			}
		}
		return true, "", nil
	}
}

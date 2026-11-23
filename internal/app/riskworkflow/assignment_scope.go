package riskworkflow

import (
	"context"
	"errors"
	"reflect"
	"strconv"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
)

type assignmentMember struct {
	LinkID, RiskID, FamilyID, OrgID             int64
	AffectedOrgID                               *int64
	LinkRevision, RiskRevision, ProposalVersion int64
	State                                       string
}

func assignmentMembers(ctx context.Context, tx pgx.Tx, ticketID int64) ([]assignmentMember, error) {
	rows, err := tx.Query(ctx, `SELECT l.id,r.id,r.family_id,r.org_id,r.affected_org_id,l.revision,r.revision,l.proposal_version,l.remediation_state
 FROM risk_ticket_links l JOIN risk_dispatch_groups g ON g.id=l.group_id JOIN risk_items r ON r.id=l.risk_id
 WHERE g.ticket_id=$1 AND g.ended_at IS NULL AND l.ended_at IS NULL ORDER BY l.id`, ticketID)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (assignmentMember, error) {
		var m assignmentMember
		err := row.Scan(&m.LinkID, &m.RiskID, &m.FamilyID, &m.OrgID, &m.AffectedOrgID, &m.LinkRevision, &m.RiskRevision, &m.ProposalVersion, &m.State)
		return m, err
	})
}

func memberOrgs(members []assignmentMember) []int64 {
	ids := []int64{}
	for _, m := range members {
		ids = append(ids, m.OrgID)
		if m.AffectedOrgID != nil {
			ids = append(ids, *m.AffectedOrgID)
		}
	}
	return ids
}

// Prelocks precede this function. The group stabilizes topology; family rows
// precede risk rows, ticket precedes links, and reminder rows are acquired later.
func (s *AssignmentService) lockAssignmentMembers(ctx context.Context, tx pgx.Tx, c AssignmentCommand, observed []assignmentMember, manage bool) ([]riskremediation.ItemResult, error) {
	var group int64
	err := tx.QueryRow(ctx, `SELECT id FROM risk_dispatch_groups WHERE ticket_id=$1 AND ended_at IS NULL FOR UPDATE`, c.TicketID).Scan(&group)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, errcode.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	members, err := assignmentMembers(ctx, tx, c.TicketID)
	if err != nil {
		return nil, err
	}
	if len(members) == 0 || len(members) > 20 || !reflect.DeepEqual(members, observed) {
		return nil, errcode.ErrConflict
	}
	risks, families, links := []int64{}, []int64{}, []int64{}
	for _, m := range members {
		risks = append(risks, m.RiskID)
		families = append(families, m.FamilyID)
		links = append(links, m.LinkID)
	}
	for _, query := range []struct {
		sql string
		ids []int64
	}{
		{`SELECT id FROM risk_families WHERE id=ANY($1) ORDER BY id FOR UPDATE`, families},
		{`SELECT id FROM risk_items WHERE id=ANY($1) ORDER BY id FOR UPDATE`, risks},
		{`SELECT id FROM tickets WHERE id=ANY($1) ORDER BY id FOR UPDATE`, []int64{c.TicketID}},
		{`SELECT id FROM risk_ticket_links WHERE id=ANY($1) ORDER BY id FOR UPDATE`, links},
	} {
		rows, e := tx.Query(ctx, query.sql, query.ids)
		if e != nil {
			return nil, e
		}
		if _, e = pgx.CollectRows(rows, pgx.RowTo[int64]); e != nil {
			return nil, e
		}
	}
	current, err := assignmentMembers(ctx, tx, c.TicketID)
	if err != nil {
		return nil, err
	}
	if !reflect.DeepEqual(current, observed) {
		return nil, errcode.ErrConflict
	}
	out := make([]riskremediation.ItemResult, 0, len(members))
	for _, m := range members {
		if err = s.assignmentRiskAccess(ctx, tx, c.ActorID, m.RiskID, manage); err != nil {
			return nil, err
		}
		var state string
		if err = tx.QueryRow(ctx, `SELECT status FROM risk_items WHERE id=$1`, m.RiskID).Scan(&state); err != nil {
			return nil, err
		}
		if state != "remediating" || m.State != "open" {
			return nil, errcode.ErrConflict
		}
		out = append(out, riskremediation.ItemResult{LinkID: m.LinkID, RiskID: m.RiskID, LinkRevision: m.LinkRevision, RiskRevision: m.RiskRevision, ProposalVersion: m.ProposalVersion, Result: "applied"})
	}
	return out, nil
}

func (s *AssignmentService) assignmentRiskAccess(ctx context.Context, tx pgx.Tx, actor, risk int64, manage bool) error {
	_, _, _, err := repository.GetRiskItemVisible(ctx, tx, risk, actor)
	if errors.Is(err, repository.ErrRiskNotFound) {
		return errcode.ErrNotFound
	}
	if err != nil {
		return err
	}
	if manage {
		allowed, e := repository.CanManageRiskItem(ctx, tx, risk, actor)
		if e != nil {
			return e
		}
		if !allowed {
			return errcode.ErrNoPermission
		}
	}
	return nil
}

// Target eligibility uses the same current DB role/policy snapshot and model as
// the admitted request. It never creates an authenticated purpose for the target.
func (s *AssignmentService) assignmentTarget(ctx context.Context, tx pgx.Tx, c AssignmentCommand, target int64, members []assignmentMember) error {
	users := repository.NewUserRepo(s.pool)
	user, err := users.FindByID(ctx, target)
	if err != nil {
		return err
	}
	if user.Status != 1 {
		return errcode.ErrUserDisabled
	}
	roles, policies, err := users.CurrentIAMAuthorization(ctx, target)
	if err != nil {
		return err
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok {
		return errcode.ErrNoPermission
	}
	m, err := casbinmodel.NewModelFromString(purpose.Model)
	if err != nil {
		return errcode.ErrNoPermission
	}
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		return errcode.ErrNoPermission
	}
	if _, err = enforcer.AddPolicies(policies); err != nil {
		return errcode.ErrNoPermission
	}
	for _, route := range []struct{ path, method string }{{acceptAssignmentPath, "POST"}, {"/api/v1/tickets/transition", "POST"}, {"/api/v1/tickets/" + strconv.FormatInt(c.TicketID, 10), "GET"}} {
		allowed := false
		for _, role := range roles {
			yes, e := enforcer.Enforce("role::"+role, route.path, route.method)
			if e != nil {
				return errcode.ErrNoPermission
			}
			allowed = allowed || yes
		}
		if !allowed {
			return errcode.ErrNoPermission
		}
	}
	// A named handler must currently qualify in every linked risk scope. This
	// check grants no new risk visibility merely because a department was chosen.
	for _, member := range members {
		if err = s.assignmentRiskAccess(ctx, tx, target, member.RiskID, false); err != nil {
			return err
		}
	}
	return nil
}

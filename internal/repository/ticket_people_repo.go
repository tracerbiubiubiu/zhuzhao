package repository

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// TicketPeopleRepo searches only the identity projection used by business pickers.
type TicketPeopleRepo struct{ db *pgxpool.Pool }

// NewTicketPeopleRepo creates the restricted search repository.
func NewTicketPeopleRepo(db *pgxpool.Pool) *TicketPeopleRepo { return &TicketPeopleRepo{db: db} }

var ticketOrgCandidateSQL = `u.status=1 AND u.deleted_at IS NULL
  AND ($2::bigint=0 OR EXISTS (
   SELECT 1 FROM user_orgs membership JOIN organizations source_org ON source_org.id=membership.org_id
   WHERE membership.user_id=u.id AND membership.org_id=$2
    AND (membership.expires_at IS NULL OR membership.expires_at>statement_timestamp())
    AND ` + OrgSourceActiveSQL("source_org") + `))`

// TransferRouteRoles are derived from current Casbin policy for this ticket.
// Different effective roles may grant detail read and transition.
type TransferRouteRoles struct {
	Read       []int64
	Transition []int64
}

func (r *TicketPeopleRepo) TransferCandidates(ctx context.Context, term string, orgID, exclude, target int64, grants TransferRouteRoles) ([]model.TicketPerson, error) {
	if len(grants.Read) == 0 || len(grants.Transition) == 0 {
		return []model.TicketPerson{}, nil
	}
	return r.searchCandidates(ctx, term, orgID, exclude, target, &grants)
}

// SearchEnabled searches enabled, undeleted candidates.
func (r *TicketPeopleRepo) SearchEnabled(ctx context.Context, term string, orgID int64) ([]model.TicketPerson, error) {
	return r.searchCandidates(ctx, term, orgID, 0, 0, nil)
}

func (r *TicketPeopleRepo) searchCandidates(ctx context.Context, term string, orgID, exclude, target int64, grants *TransferRouteRoles) ([]model.TicketPerson, error) {
	// Treat wildcard characters as literal input. Searching never exposes full user rows.
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term) + "%"
	query := `SELECT u.id,
	 COALESCE(NULLIF(u.real_name,''),u.username) || '(' || COALESCE(u.employee_no,'') || ')'
	 FROM users u WHERE ` + ticketOrgCandidateSQL + ` AND u.id<>$3
	 AND (u.real_name ILIKE $1 ESCAPE '\' OR u.username ILIKE $1 ESCAPE '\' OR u.employee_no ILIKE $1 ESCAPE '\')`
	args := []any{pattern, orgID, exclude}
	if grants != nil {
		query += ` AND ($4::bigint=0 OR u.id=$4) AND (` + effectiveRoleCTEFor("u.id") + `
    SELECT bool_or(id=ANY($5::bigint[])) AND bool_or(id=ANY($6::bigint[])) FROM expanded)`
		args = append(args, target, grants.Read, grants.Transition)
	}
	query += " ORDER BY COALESCE(NULLIF(u.real_name,''),u.username),u.id LIMIT 20"
	rows, err := IAMDB(ctx, r.db).Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.TicketPerson, error) {
		var p model.TicketPerson
		err := row.Scan(&p.ID, &p.Label)
		return p, err
	})
	if list == nil && err == nil {
		list = []model.TicketPerson{}
	}
	return list, err
}

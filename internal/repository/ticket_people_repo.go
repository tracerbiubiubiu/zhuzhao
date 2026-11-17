package repository

import (
	"context"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"strings"
)

// TicketPeopleRepo searches only the identity projection used by business pickers.
type TicketPeopleRepo struct{ db *pgxpool.Pool }

// NewTicketPeopleRepo creates the restricted search repository.
func NewTicketPeopleRepo(db *pgxpool.Pool) *TicketPeopleRepo { return &TicketPeopleRepo{db: db} }

// SearchEnabled searches enabled, undeleted candidates; orgID is rule-target only.
func (r *TicketPeopleRepo) SearchEnabled(ctx context.Context, term string, orgID int64) ([]model.TicketPerson, error) {
	// Treat wildcard characters as literal input. Searching never exposes full user rows.
	pattern := "%" + strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(term) + "%"
	rows, err := r.db.Query(ctx, `SELECT u.id,
	 COALESCE(NULLIF(u.real_name,''),u.username) || '(' || COALESCE(u.employee_no,'') || ')'
	 FROM users u WHERE u.status=1 AND u.deleted_at IS NULL
	 AND (u.real_name ILIKE $1 ESCAPE '\' OR u.username ILIKE $1 ESCAPE '\' OR u.employee_no ILIKE $1 ESCAPE '\')
	 AND ($2::bigint=0 OR EXISTS (
	  SELECT 1 FROM user_orgs membership JOIN organizations source_org ON source_org.id=membership.org_id
	  WHERE membership.user_id=u.id AND membership.org_id=$2
	  AND (membership.expires_at IS NULL OR membership.expires_at>NOW())
	  AND `+OrgSourceActiveSQL("source_org")+`))
	 ORDER BY COALESCE(NULLIF(u.real_name,''),u.username),u.id LIMIT 20`, pattern, orgID)
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

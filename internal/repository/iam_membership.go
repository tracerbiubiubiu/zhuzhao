package repository

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// PotentialMembershipRoles returns the bound roles a NEW membership can supply
// after future reactivation. Status/expiry are not a way to plant a strong grant.
// Existing membership facts, including expired/managed ones, are not regranted.
func (r *OrgRepo) PotentialMembershipRoles(ctx context.Context, userID int64, orgIDs []int64) ([]*model.Role, error) {
	rows, err := IAMDB(ctx, r.db).Query(ctx, `WITH RECURSIVE potential(id) AS (
 SELECT g.role_id FROM org_roles g WHERE g.org_id=ANY($2::bigint[])
 AND NOT EXISTS(SELECT 1 FROM user_orgs retained WHERE retained.org_id=g.org_id AND retained.user_id=$1)
 UNION
 SELECT r.parent_id FROM roles r JOIN potential p ON r.id=p.id WHERE r.parent_id IS NOT NULL
 ) SELECT r.id,r.code,r.priority FROM roles r JOIN potential p ON p.id=r.id ORDER BY r.id`, userID, orgIDs)
	if err != nil {
		return nil, fmt.Errorf("read new membership role bindings: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.Role, error) {
		r := &model.Role{}
		err := row.Scan(&r.ID, &r.Code, &r.Priority)
		return r, err
	})
}

// MembershipRolesForRemoval checks the authority attached to a fact actually
// being removed. An owner-only reference without membership grants no L1 role.
func (r *OrgRepo) MembershipRolesForRemoval(ctx context.Context, userID, orgID int64) ([]*model.Role, error) {
	rows, err := IAMDB(ctx, r.db).Query(ctx, `WITH RECURSIVE potential(id) AS (
 SELECT g.role_id FROM org_roles g JOIN user_orgs m ON m.org_id=g.org_id
 WHERE m.user_id=$1 AND m.org_id=$2
 UNION SELECT r.parent_id FROM roles r JOIN potential p ON p.id=r.id WHERE r.parent_id IS NOT NULL
 ) SELECT r.id,r.code,r.priority FROM roles r JOIN potential p ON p.id=r.id ORDER BY r.id`, userID, orgID)
	if err != nil {
		return nil, fmt.Errorf("read removed membership roles: %w", err)
	}
	return pgx.CollectRows(rows, scanPotentialRole)
}

// PotentialSubtreeRoles follows actual parent IDs. Paused organizations and
// disabled roles can contribute after reactivation. Soft-deleted roles cannot.
func (r *OrgRepo) PotentialSubtreeRoles(ctx context.Context, orgID int64) ([]*model.Role, error) {
	rows, err := IAMDB(ctx, r.db).Query(ctx, `WITH RECURSIVE subtree(id) AS (
 SELECT id FROM organizations WHERE id=$1
 UNION SELECT child.id FROM organizations child JOIN subtree parent ON child.parent_id=parent.id
 ),potential(id) AS (
 SELECT g.role_id FROM org_roles g JOIN subtree s ON s.id=g.org_id
 JOIN organizations o ON o.id=s.id AND o.deleted_at IS NULL
 JOIN roles bound ON bound.id=g.role_id AND bound.deleted_at IS NULL
 UNION SELECT parent.id FROM potential p JOIN roles child ON child.id=p.id
 JOIN roles parent ON parent.id=child.parent_id AND parent.deleted_at IS NULL
 ) SELECT r.id,r.code,r.priority FROM roles r JOIN potential p ON p.id=r.id ORDER BY r.id`, orgID)
	if err != nil {
		return nil, fmt.Errorf("read changed organization source roles: %w", err)
	}
	return pgx.CollectRows(rows, scanPotentialRole)
}

func scanPotentialRole(row pgx.CollectableRow) (*model.Role, error) {
	role := &model.Role{}
	err := row.Scan(&role.ID, &role.Code, &role.Priority)
	return role, err
}

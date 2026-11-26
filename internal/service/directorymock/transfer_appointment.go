package directorymock

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func transferAppointment(ctx context.Context, db repository.IAMDBTX, orgs *repository.OrgRepo, event PrimaryTransfer) (string, error) {
	var userID, tenantID, revision int64
	err := db.QueryRow(ctx, `SELECT u.id,u.tenant_id,b.source_revision
		FROM directory_mock_employee_bindings b JOIN users u ON u.id=b.user_id
		WHERE b.scope_ref=$1 AND b.external_id=$2 AND b.identity_generation=$3
		AND u.source=$4 AND u.is_system IS FALSE AND u.status=1 AND u.deleted_at IS NULL`,
		event.ScopeRef, event.EmployeeExternalID, event.IdentityGeneration, repository.MockDirectorySource).
		Scan(&userID, &tenantID, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return "employee_unresolved", nil
	}
	if err != nil {
		return "", fmt.Errorf("resolve mock employee: %w", err)
	}
	if event.SourceRevision <= revision {
		return "source_revision_not_advanced", nil
	}

	to, err := resolveTransferDepartment(ctx, db, event.ScopeRef, event.ToDepartmentExternalID, userID, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "target_department_unresolved", nil
	}
	if err != nil {
		return "", err
	}
	from, err := resolveTransferDepartment(ctx, db, event.ScopeRef, event.FromDepartmentExternalID, userID, tenantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "source_department_unresolved", nil
	}
	if err != nil {
		return "", err
	}
	var canRemove, targetMember bool
	if err := db.QueryRow(ctx, `SELECT EXISTS(
		SELECT 1 FROM user_orgs WHERE user_id=$1 AND org_id=$2 AND is_primary
		AND source=$4 AND org_member_role='member' AND ticket_scope='assigned' AND expires_at IS NULL
	), EXISTS(SELECT 1 FROM user_orgs WHERE user_id=$1 AND org_id=$3)`,
		userID, from.id, to.id, repository.MockDirectorySource).Scan(&canRemove, &targetMember); err != nil {
		return "", fmt.Errorf("check mock primary appointment: %w", err)
	}
	if !canRemove || from.owner {
		return "source_primary_not_transferable", nil
	}
	if targetMember || to.owner {
		return "target_appointment_conflict", nil
	}
	sourceRoles, err := orgs.MembershipRolesForRemoval(ctx, userID, from.id)
	if err != nil {
		return "", err
	}
	if len(sourceRoles) != 0 {
		return "source_membership_role_grant", nil
	}
	roles, err := orgs.PotentialMembershipRoles(ctx, userID, []int64{to.id})
	if err != nil {
		return "", err
	}
	if len(roles) != 0 {
		return "target_membership_role_grant", nil
	}
	return "", movePrimaryAppointment(ctx, db, event, userID, from.id, to.id)
}

type transferDepartment struct {
	id    int64
	owner bool
}

func resolveTransferDepartment(ctx context.Context, db repository.IAMDBTX, scope, externalID string, userID, tenantID int64) (transferDepartment, error) {
	var dept transferDepartment
	err := db.QueryRow(ctx, `SELECT o.id,COALESCE($3=ANY(o.owner_user_ids),false)
		FROM directory_mock_department_bindings b JOIN organizations o ON o.id=b.org_id
		WHERE b.scope_ref=$1 AND b.external_id=$2 AND o.tenant_id=$4 AND o.source=$5
		AND NOT o.is_virtual AND o.is_system IS FALSE AND (`+repository.OrgSourceActiveSQL("o")+`)`,
		scope, externalID, userID, tenantID, repository.MockDirectorySource).Scan(&dept.id, &dept.owner)
	if err != nil {
		return transferDepartment{}, fmt.Errorf("resolve mock department: %w", err)
	}
	return dept, nil
}

func movePrimaryAppointment(ctx context.Context, db repository.IAMDBTX, event PrimaryTransfer, userID, fromID, toID int64) error {
	if _, err := db.Exec(ctx, `DELETE FROM user_orgs WHERE user_id=$1 AND org_id=$2`, userID, fromID); err != nil {
		return fmt.Errorf("remove mock source appointment: %w", err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,is_primary,org_member_role,ticket_scope,source)
		VALUES ($1,$2,true,'member','assigned',$3)`, userID, toID, repository.MockDirectorySource); err != nil {
		return fmt.Errorf("write mock target appointment: %w", err)
	}
	if _, err := db.Exec(ctx, `UPDATE directory_mock_employee_bindings SET source_revision=$1
		WHERE scope_ref=$2 AND external_id=$3 AND identity_generation=$4`,
		event.SourceRevision, event.ScopeRef, event.EmployeeExternalID, event.IdentityGeneration); err != nil {
		return fmt.Errorf("advance mock employee revision: %w", err)
	}
	return nil
}

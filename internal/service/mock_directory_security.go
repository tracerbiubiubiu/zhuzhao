package service

import (
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const sessionCmdMockDirectoryDisable = "mock.directory.disable"

type MockDirectoryTermination struct {
	ScopeRef           string
	EmployeeExternalID string
	IdentityGeneration int64
	SourceRevision     int64
}

// RecordMockDirectoryDisable joins a scoped directory event to the C05 session
// safety path. The running Core worker owns projection after commit.
func RecordMockDirectoryDisable(ctx context.Context, pool *pgxpool.Pool, event MockDirectoryTermination,
	operationID string) (int64, string, error) {
	if !repository.IAMWriteActive(ctx) {
		return 0, "", errors.New("directory lifecycle requires the IAM write guard")
	}
	if _, err := uuid.Parse(operationID); err != nil {
		return 0, "", errors.New("directory lifecycle requires a directory run ID")
	}
	db := repository.IAMDB(ctx, pool)
	var userID, revision int64
	err := db.QueryRow(ctx, `SELECT u.id,b.source_revision
		FROM directory_mock_employee_bindings b JOIN users u ON u.id=b.user_id
		WHERE b.scope_ref=$1 AND b.external_id=$2 AND b.identity_generation=$3
		  AND u.source=$4 AND u.is_system IS FALSE AND u.deleted_at IS NULL`,
		event.ScopeRef, event.EmployeeExternalID, event.IdentityGeneration, repository.MockDirectorySource).Scan(&userID, &revision)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, "employee_unresolved", nil
	}
	if err != nil {
		return 0, "", fmt.Errorf("resolve mock employee disable: %w", err)
	}
	if event.SourceRevision <= revision {
		return 0, "source_revision_not_advanced", nil
	}
	roles, err := repository.NewRoleRepo(pool).GetEffectiveRoleCodes(ctx, userID)
	if err != nil {
		return 0, "", err
	}
	if slices.Contains(roles, "superadmin") || slices.Contains(roles, "admin") {
		return 0, "protected_account", nil
	}
	userRepo := repository.NewUserRepo(pool)
	deps := NewSessionWorkerDeps(pool, userRepo, nil, 0)
	epoch, _, _, err := securityWrite(ctx, deps, sessionCmdMockDirectoryDisable, operationID, repository.MockDirectorySource, 0, userID,
		func(snap *repository.UserSecuritySnapshot) error {
			if snap.Deleted {
				return errcode.ErrUserNotFound
			}
			return nil
		},
		func(tx pgx.Tx, snap *repository.UserSecuritySnapshot) (int64, bool, error) {
			if snap.Status == 0 {
				return snap.SessionEpoch, false, nil
			}
			return userRepo.AdvanceEpochSetStatusTx(ctx, tx, userID, snap.SessionEpoch, 0)
		},
		func(*repository.UserSecuritySnapshot) bool { return false }, "mock-directory-terminated")
	if err != nil {
		return 0, "", err
	}
	if _, err := db.Exec(ctx, `UPDATE directory_mock_employee_bindings
		SET source_revision=$1,source_terminated=true WHERE user_id=$2`, event.SourceRevision, userID); err != nil {
		return 0, "", fmt.Errorf("write mock lifecycle fact: %w", err)
	}
	return epoch, "", nil
}

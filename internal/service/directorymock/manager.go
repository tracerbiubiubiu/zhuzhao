package directorymock

import (
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	StatusManagerResolved = "resolved"
	statusManagerPending  = "unresolved"
	statusManagerRejected = "rejected"
	maxManagerBytes       = 4096
)

var (
	ErrInvalidManager       = errors.New("invalid_mock_manager_resolution")
	ErrManagerConflict      = errors.New("mock_manager_content_conflict")
	ErrManagerReceiptAbsent = errors.New("mock_manager_receipt_not_found")
)

type ManagerResolution struct {
	ProviderRef               string `json:"provider_ref"`
	ScopeRef                  string `json:"scope_ref"`
	OperationID               string `json:"operation_id"`
	EmployeeExternalID        string `json:"employee_external_id"`
	IdentityGeneration        int64  `json:"identity_generation"`
	ManagerExternalID         string `json:"manager_external_id"`
	ManagerIdentityGeneration int64  `json:"manager_identity_generation"`
}

type ManagerReceipt struct {
	RunID                     string `json:"run_id"`
	ScopeRef                  string `json:"scope_ref"`
	OperationID               string `json:"operation_id"`
	Status                    string `json:"status"`
	EmployeeExternalID        string `json:"employee_external_id"`
	IdentityGeneration        int64  `json:"identity_generation"`
	ManagerExternalID         string `json:"manager_external_id"`
	ManagerIdentityGeneration int64  `json:"manager_identity_generation"`
	Problem                   string `json:"problem,omitempty"`
}

func ReadManagerResolution(input io.Reader) (ManagerResolution, error) {
	var event ManagerResolution
	reader := &io.LimitedReader{R: input, N: maxManagerBytes + 1}
	if err := jsonv2.UnmarshalRead(reader, &event, jsonv2.RejectUnknownMembers(true)); err != nil ||
		reader.N == 0 || !event.valid() {
		return ManagerResolution{}, ErrInvalidManager
	}
	return event, nil
}

func (event ManagerResolution) valid() bool {
	return event.ProviderRef == repository.MockDirectorySource && validRef(event.ScopeRef) &&
		validRef(event.OperationID) && validRef(event.EmployeeExternalID) && validRef(event.ManagerExternalID) &&
		event.IdentityGeneration > 0 && event.ManagerIdentityGeneration > 0
}

// A slash is outside validRef, so the legacy transfer command can neither
// create nor see a manager operation key even in a database with newer runs.
func managerOperationKey(scope, operation string) operationKey {
	return operationKey{scope: scope, id: "manager/" + operation}
}

// ResolveManager records one historical observation without updating IAM facts.
func ResolveManager(ctx context.Context, pool *pgxpool.Pool, event ManagerResolution) (ManagerReceipt, error) {
	if !event.valid() {
		return ManagerReceipt{}, ErrInvalidManager
	}
	key := managerOperationKey(event.ScopeRef, event.OperationID)
	receipt, err := recordOperation(ctx, pool, key, event, func(ctx context.Context, db repository.IAMDBTX, runID string) (ManagerReceipt, error) {
		status, problem, err := managerOutcome(ctx, db, event)
		if err != nil {
			return ManagerReceipt{}, err
		}
		return ManagerReceipt{
			RunID: runID, ScopeRef: event.ScopeRef, OperationID: event.OperationID, Status: status,
			EmployeeExternalID: event.EmployeeExternalID, IdentityGeneration: event.IdentityGeneration,
			ManagerExternalID: event.ManagerExternalID, ManagerIdentityGeneration: event.ManagerIdentityGeneration,
			Problem: problem,
		}, nil
	})
	switch {
	case errors.Is(err, errScopeAbsent):
		err = ErrInvalidManager
	case errors.Is(err, errOperationConflict):
		err = ErrManagerConflict
	}
	return receipt, err
}

func LookupManagerReceipt(ctx context.Context, pool *pgxpool.Pool, scope, operation string) (ManagerReceipt, error) {
	if !validRef(scope) || !validRef(operation) {
		return ManagerReceipt{}, ErrInvalidManager
	}
	receipt, _, err := readOperationReceipt[ManagerReceipt](ctx, pool, managerOperationKey(scope, operation))
	if errors.Is(err, errOperationAbsent) {
		err = ErrManagerReceiptAbsent
	}
	return receipt, err
}

func managerOutcome(ctx context.Context, db repository.IAMDBTX, event ManagerResolution) (status, problem string, err error) {
	employee, err := readMockEmployeeIdentity(ctx, db, event.ScopeRef, event.EmployeeExternalID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && (!employee.eligible || employee.generation != event.IdentityGeneration)) {
		return statusManagerRejected, "employee_unresolved", nil
	}
	if err != nil {
		return "", "", err
	}
	if event.EmployeeExternalID == event.ManagerExternalID && event.IdentityGeneration == event.ManagerIdentityGeneration {
		return statusManagerRejected, "manager_self_reference", nil
	}
	manager, err := readMockEmployeeIdentity(ctx, db, event.ScopeRef, event.ManagerExternalID)
	if errors.Is(err, pgx.ErrNoRows) {
		return statusManagerPending, "manager_not_registered", nil
	}
	if err != nil {
		return "", "", err
	}
	if !manager.eligible || manager.generation != event.ManagerIdentityGeneration || manager.tenant != employee.tenant {
		return statusManagerRejected, "manager_link_invalid", nil
	}
	return StatusManagerResolved, "", nil
}

type mockEmployeeIdentity struct {
	tenant     int64
	generation int64
	eligible   bool
}

func readMockEmployeeIdentity(ctx context.Context, db repository.IAMDBTX, scope, externalID string) (mockEmployeeIdentity, error) {
	var identity mockEmployeeIdentity
	err := db.QueryRow(ctx, `SELECT u.tenant_id,b.identity_generation,
		u.source=$3 AND u.status=1 AND u.deleted_at IS NULL AND u.is_system IS FALSE
		FROM directory_mock_employee_bindings b JOIN users u ON u.id=b.user_id
		WHERE b.scope_ref=$1 AND b.external_id=$2`, scope, externalID, repository.MockDirectorySource).
		Scan(&identity.tenant, &identity.generation, &identity.eligible)
	if err != nil {
		return mockEmployeeIdentity{}, fmt.Errorf("read mock employee identity: %w", err)
	}
	return identity, nil
}

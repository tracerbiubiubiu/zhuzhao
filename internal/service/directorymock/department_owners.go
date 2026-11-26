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
	StatusDepartmentOwnersResolved    = "resolved"
	statusDepartmentOwnersUnavailable = "unavailable"
	maxDepartmentOwnerBytes           = 16 << 10
	maxDepartmentOwnerCandidates      = 64
)

var (
	ErrInvalidDepartmentOwners       = errors.New("invalid_mock_department_owner_resolution")
	ErrDepartmentOwnersConflict      = errors.New("mock_department_owner_content_conflict")
	ErrDepartmentOwnersReceiptAbsent = errors.New("mock_department_owner_receipt_not_found")
)

type DepartmentOwnerCandidate struct {
	EmployeeExternalID string `json:"employee_external_id"`
	IdentityGeneration int64  `json:"identity_generation"`
}

type DepartmentOwnerResolution struct {
	ProviderRef          string                     `json:"provider_ref"`
	ScopeRef             string                     `json:"scope_ref"`
	OperationID          string                     `json:"operation_id"`
	DepartmentExternalID string                     `json:"department_external_id"`
	Candidates           []DepartmentOwnerCandidate `json:"candidates"`
}

type DepartmentOwnerCandidateResult struct {
	DepartmentOwnerCandidate
	Status  string `json:"status"`
	Problem string `json:"problem,omitempty"`
}

type DepartmentOwnerReceipt struct {
	RunID                string                           `json:"run_id"`
	ScopeRef             string                           `json:"scope_ref"`
	OperationID          string                           `json:"operation_id"`
	DepartmentExternalID string                           `json:"department_external_id"`
	Status               string                           `json:"status"`
	Problem              string                           `json:"problem,omitempty"`
	Candidates           []DepartmentOwnerCandidateResult `json:"candidates"`
}

func ReadDepartmentOwnerResolution(input io.Reader) (DepartmentOwnerResolution, error) {
	var event DepartmentOwnerResolution
	reader := &io.LimitedReader{R: input, N: maxDepartmentOwnerBytes + 1}
	if err := jsonv2.UnmarshalRead(reader, &event, jsonv2.RejectUnknownMembers(true)); err != nil ||
		reader.N == 0 || !event.valid() {
		return DepartmentOwnerResolution{}, ErrInvalidDepartmentOwners
	}
	return event, nil
}

func (event DepartmentOwnerResolution) valid() bool {
	if event.ProviderRef != repository.MockDirectorySource || !validRef(event.ScopeRef) ||
		!validRef(event.OperationID) || !validRef(event.DepartmentExternalID) || event.Candidates == nil ||
		len(event.Candidates) > maxDepartmentOwnerCandidates {
		return false
	}
	seen := make(map[string]struct{}, len(event.Candidates))
	for _, candidate := range event.Candidates {
		if _, duplicate := seen[candidate.EmployeeExternalID]; duplicate ||
			!validRef(candidate.EmployeeExternalID) || candidate.IdentityGeneration <= 0 {
			return false
		}
		seen[candidate.EmployeeExternalID] = struct{}{}
	}
	return true
}

// The slash keeps public operation IDs independent from transfer and manager receipts.
func departmentOwnerOperationKey(scope, operation string) operationKey {
	return operationKey{scope: scope, id: "department-owners/" + operation}
}

// ResolveDepartmentOwners records explicit directory observations without changing IAM facts.
func ResolveDepartmentOwners(ctx context.Context, pool *pgxpool.Pool, event DepartmentOwnerResolution) (DepartmentOwnerReceipt, error) {
	if !event.valid() {
		return DepartmentOwnerReceipt{}, ErrInvalidDepartmentOwners
	}
	key := departmentOwnerOperationKey(event.ScopeRef, event.OperationID)
	receipt, err := recordOperation(ctx, pool, key, event, func(ctx context.Context, db repository.IAMDBTX, runID string) (DepartmentOwnerReceipt, error) {
		return departmentOwnerOutcome(ctx, db, event, runID)
	})
	switch {
	case errors.Is(err, errScopeAbsent):
		err = ErrInvalidDepartmentOwners
	case errors.Is(err, errOperationConflict):
		err = ErrDepartmentOwnersConflict
	}
	return receipt, err
}

func LookupDepartmentOwnerReceipt(ctx context.Context, pool *pgxpool.Pool, scope, operation string) (DepartmentOwnerReceipt, error) {
	if !validRef(scope) || !validRef(operation) {
		return DepartmentOwnerReceipt{}, ErrInvalidDepartmentOwners
	}
	receipt, _, err := readOperationReceipt[DepartmentOwnerReceipt](ctx, pool, departmentOwnerOperationKey(scope, operation))
	if errors.Is(err, errOperationAbsent) {
		err = ErrDepartmentOwnersReceiptAbsent
	}
	return receipt, err
}

func departmentOwnerOutcome(ctx context.Context, db repository.IAMDBTX, event DepartmentOwnerResolution, runID string) (DepartmentOwnerReceipt, error) {
	receipt := DepartmentOwnerReceipt{
		RunID: runID, ScopeRef: event.ScopeRef, OperationID: event.OperationID, DepartmentExternalID: event.DepartmentExternalID,
		Status:     statusDepartmentOwnersUnavailable,
		Candidates: make([]DepartmentOwnerCandidateResult, 0, len(event.Candidates)),
	}
	var tenant int64
	err := db.QueryRow(ctx, `SELECT o.tenant_id
		FROM directory_mock_department_bindings b JOIN organizations o ON o.id=b.org_id
		WHERE b.scope_ref=$1 AND b.external_id=$2 AND o.source=$3 AND NOT o.is_virtual
		AND o.is_system IS FALSE AND (`+repository.OrgSourceActiveSQL("o")+`)`,
		event.ScopeRef, event.DepartmentExternalID, repository.MockDirectorySource).Scan(&tenant)
	if errors.Is(err, pgx.ErrNoRows) {
		receipt.Problem = "department_unresolved"
		return receipt, nil
	}
	if err != nil {
		return DepartmentOwnerReceipt{}, fmt.Errorf("resolve mock owner department: %w", err)
	}
	if len(event.Candidates) == 0 {
		receipt.Problem = "owners_not_supplied"
		return receipt, nil
	}
	receipt.Problem = "no_viable_owner"
	for _, candidate := range event.Candidates {
		identity, err := readMockEmployeeIdentity(ctx, db, event.ScopeRef, candidate.EmployeeExternalID)
		result := DepartmentOwnerCandidateResult{DepartmentOwnerCandidate: candidate}
		switch {
		case errors.Is(err, pgx.ErrNoRows):
			result.Status, result.Problem = "missing", "owner_not_registered"
		case err != nil:
			return DepartmentOwnerReceipt{}, err
		case !identity.eligible || identity.generation != candidate.IdentityGeneration || identity.tenant != tenant:
			result.Status, result.Problem = "invalid", "owner_link_invalid"
		default:
			result.Status = StatusDepartmentOwnersResolved
			receipt.Status, receipt.Problem = StatusDepartmentOwnersResolved, ""
		}
		receipt.Candidates = append(receipt.Candidates, result)
	}
	return receipt, nil
}

package directorymock

import (
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	StatusTransferApplied = "applied"
	statusTransferHeld    = "held"
	maxTransferBytes      = 4096
)

var (
	ErrInvalidTransfer       = errors.New("invalid_mock_transfer")
	ErrTransferConflict      = errors.New("mock_transfer_content_conflict")
	ErrTransferReceiptAbsent = errors.New("mock_transfer_receipt_not_found")
)

type PrimaryTransfer struct {
	ProviderRef              string `json:"provider_ref"`
	ScopeRef                 string `json:"scope_ref"`
	OperationID              string `json:"operation_id"`
	EmployeeExternalID       string `json:"employee_external_id"`
	IdentityGeneration       int64  `json:"identity_generation"`
	FromDepartmentExternalID string `json:"from_department_external_id"`
	ToDepartmentExternalID   string `json:"to_department_external_id"`
	SourceRevision           int64  `json:"source_revision"`
}

type TransferReceipt struct {
	RunID          string `json:"run_id"`
	ScopeRef       string `json:"scope_ref"`
	OperationID    string `json:"operation_id"`
	Status         string `json:"status"`
	SourceRevision int64  `json:"source_revision"`
	Problem        string `json:"problem,omitempty"`
}

// ReadPrimaryTransfer accepts one bounded synthetic event with external references.
func ReadPrimaryTransfer(input io.Reader) (PrimaryTransfer, error) {
	var event PrimaryTransfer
	reader := &io.LimitedReader{R: input, N: maxTransferBytes + 1}
	if err := jsonv2.UnmarshalRead(reader, &event, jsonv2.RejectUnknownMembers(true)); err != nil ||
		reader.N == 0 || !event.valid() {
		return PrimaryTransfer{}, ErrInvalidTransfer
	}
	return event, nil
}

func (event PrimaryTransfer) valid() bool {
	return event.ProviderRef == repository.MockDirectorySource &&
		validRef(event.ScopeRef) && validRef(event.OperationID) && validRef(event.EmployeeExternalID) &&
		validRef(event.FromDepartmentExternalID) && validRef(event.ToDepartmentExternalID) &&
		event.FromDepartmentExternalID != event.ToDepartmentExternalID &&
		event.IdentityGeneration > 0 && event.SourceRevision > 0
}

// ApplyPrimaryTransfer commits the IAM appointments, source revision and receipt
// together under the same guard used by local membership writers.
func ApplyPrimaryTransfer(ctx context.Context, pool *pgxpool.Pool, event PrimaryTransfer) (TransferReceipt, error) {
	if !event.valid() {
		return TransferReceipt{}, ErrInvalidTransfer
	}
	key := operationKey{scope: event.ScopeRef, id: event.OperationID}
	receipt, err := recordOperation(ctx, pool, key, event, func(ctx context.Context, db repository.IAMDBTX, runID string) (TransferReceipt, error) {
		receipt := TransferReceipt{
			RunID: runID, ScopeRef: event.ScopeRef, OperationID: event.OperationID,
			SourceRevision: event.SourceRevision, Status: StatusTransferApplied,
		}
		problem, err := transferAppointment(ctx, db, repository.NewOrgRepo(pool), event)
		if err != nil {
			return TransferReceipt{}, err
		}
		if problem != "" {
			receipt.Status, receipt.Problem = statusTransferHeld, problem
		}
		return receipt, nil
	})
	switch {
	case errors.Is(err, errScopeAbsent):
		err = ErrInvalidTransfer
	case errors.Is(err, errOperationConflict):
		err = ErrTransferConflict
	}
	return receipt, err
}

// LookupTransferReceipt reads a committed receipt without resubmitting the event.
func LookupTransferReceipt(ctx context.Context, pool *pgxpool.Pool, scope, operation string) (TransferReceipt, error) {
	if !validRef(scope) || !validRef(operation) {
		return TransferReceipt{}, ErrInvalidTransfer
	}
	receipt, _, err := readOperationReceipt[TransferReceipt](ctx, pool, operationKey{scope: scope, id: operation})
	if errors.Is(err, errOperationAbsent) {
		err = ErrTransferReceiptAbsent
	}
	return receipt, err
}

package directorymock

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"fmt"
	"io"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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
	encoded, err := json.Marshal(event)
	if err != nil {
		return TransferReceipt{}, fmt.Errorf("encode mock transfer: %w", err)
	}
	digest := sha256.Sum256(encoded)
	hash := hex.EncodeToString(digest[:])
	return repository.WithIAMGuardValue(ctx, pool, func(ctx context.Context) (TransferReceipt, error) {
		db := repository.IAMDB(ctx, pool)
		stored, storedHash, err := readTransferReceipt(ctx, db, event.ScopeRef, event.OperationID)
		switch {
		case err == nil && storedHash == hash:
			return stored, nil
		case err == nil:
			return TransferReceipt{}, ErrTransferConflict
		case !errors.Is(err, ErrTransferReceiptAbsent):
			return TransferReceipt{}, err
		}

		var knownScope bool
		if err := db.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM directory_mock_scopes WHERE scope_ref=$1)", event.ScopeRef).
			Scan(&knownScope); err != nil {
			return TransferReceipt{}, fmt.Errorf("resolve mock scope: %w", err)
		}
		if !knownScope {
			return TransferReceipt{}, ErrInvalidTransfer
		}
		receipt := TransferReceipt{
			RunID: uuid.NewString(), ScopeRef: event.ScopeRef, OperationID: event.OperationID,
			SourceRevision: event.SourceRevision, Status: StatusTransferApplied,
		}
		problem, err := transferAppointment(ctx, db, repository.NewOrgRepo(pool), event)
		if err != nil {
			return TransferReceipt{}, err
		}
		if problem != "" {
			receipt.Status, receipt.Problem = statusTransferHeld, problem
		}
		if _, err := db.Exec(ctx, `INSERT INTO directory_mock_transfer_runs(scope_ref,operation_id,run_id,input_hash,receipt)
			VALUES ($1,$2,$3,$4,$5)`, receipt.ScopeRef, receipt.OperationID, receipt.RunID, hash, receipt); err != nil {
			return TransferReceipt{}, fmt.Errorf("save mock transfer receipt: %w", err)
		}
		return receipt, nil
	})
}

// LookupTransferReceipt reads a committed receipt without resubmitting the event.
func LookupTransferReceipt(ctx context.Context, pool *pgxpool.Pool, scope, operation string) (TransferReceipt, error) {
	if !validRef(scope) || !validRef(operation) {
		return TransferReceipt{}, ErrInvalidTransfer
	}
	receipt, _, err := readTransferReceipt(ctx, pool, scope, operation)
	return receipt, err
}

func readTransferReceipt(ctx context.Context, db repository.IAMDBTX, scope, operation string) (TransferReceipt, string, error) {
	var receipt TransferReceipt
	var hash string
	err := db.QueryRow(ctx, `SELECT receipt,input_hash FROM directory_mock_transfer_runs WHERE scope_ref=$1 AND operation_id=$2`,
		scope, operation).Scan(&receipt, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return TransferReceipt{}, "", ErrTransferReceiptAbsent
	}
	if err != nil {
		return TransferReceipt{}, "", fmt.Errorf("read mock transfer receipt: %w", err)
	}
	return receipt, hash, nil
}

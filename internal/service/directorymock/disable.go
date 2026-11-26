package directorymock

import (
	"context"
	jsonv2 "encoding/json/v2"
	"errors"
	"io"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

const StatusEmployeeDisabled = "disabled"

var (
	ErrInvalidDisable       = errors.New("invalid_mock_disable")
	ErrDisableConflict      = errors.New("mock_disable_content_conflict")
	ErrDisableReceiptAbsent = errors.New("mock_disable_receipt_not_found")
)

type EmployeeDisable struct {
	ProviderRef        string `json:"provider_ref"`
	ScopeRef           string `json:"scope_ref"`
	OperationID        string `json:"operation_id"`
	EmployeeExternalID string `json:"employee_external_id"`
	IdentityGeneration int64  `json:"identity_generation"`
	SourceRevision     int64  `json:"source_revision"`
	Lifecycle          string `json:"lifecycle"`
}

type DisableReceipt struct {
	RunID              string `json:"run_id"`
	ScopeRef           string `json:"scope_ref"`
	OperationID        string `json:"operation_id"`
	EmployeeExternalID string `json:"employee_external_id"`
	IdentityGeneration int64  `json:"identity_generation"`
	SourceRevision     int64  `json:"source_revision"`
	Status             string `json:"status"`
	Problem            string `json:"problem,omitempty"`
	SessionEpoch       int64  `json:"session_epoch,omitempty"`
}

type DisableReceiptView struct {
	DisableReceipt
	RevocationStatus string `json:"revocation_status,omitempty"`
}

func ReadEmployeeDisable(input io.Reader) (EmployeeDisable, error) {
	var event EmployeeDisable
	reader := &io.LimitedReader{R: input, N: 4097}
	if err := jsonv2.UnmarshalRead(reader, &event, jsonv2.RejectUnknownMembers(true)); err != nil ||
		reader.N == 0 || !event.valid() {
		return EmployeeDisable{}, ErrInvalidDisable
	}
	return event, nil
}

func (event EmployeeDisable) valid() bool {
	return event.ProviderRef == repository.MockDirectorySource && event.Lifecycle == "terminated" &&
		validRef(event.ScopeRef) && validRef(event.OperationID) && validRef(event.EmployeeExternalID) &&
		event.IdentityGeneration > 0 && event.SourceRevision > 0
}

func disableOperationKey(scope, operation string) operationKey {
	return operationKey{scope: scope, id: "disable/" + operation}
}

// DisableEmployee applies only an explicit, monotonic event. The durable C05
// operation and intent commit in the same IAM transaction as the mock receipt.
func DisableEmployee(ctx context.Context, pool *pgxpool.Pool, event EmployeeDisable) (DisableReceipt, error) {
	if !event.valid() {
		return DisableReceipt{}, ErrInvalidDisable
	}
	receipt, err := recordOperation(ctx, pool, disableOperationKey(event.ScopeRef, event.OperationID), event,
		func(ctx context.Context, _ repository.IAMDBTX, runID string) (DisableReceipt, error) {
			result := DisableReceipt{
				RunID: runID, ScopeRef: event.ScopeRef, OperationID: event.OperationID,
				EmployeeExternalID: event.EmployeeExternalID, IdentityGeneration: event.IdentityGeneration,
				SourceRevision: event.SourceRevision, Status: StatusEmployeeDisabled,
			}
			epoch, problem, err := service.RecordMockDirectoryDisable(ctx, pool, service.MockDirectoryTermination{
				ScopeRef: event.ScopeRef, EmployeeExternalID: event.EmployeeExternalID,
				IdentityGeneration: event.IdentityGeneration, SourceRevision: event.SourceRevision,
			}, runID)
			if err != nil {
				return DisableReceipt{}, err
			}
			result.SessionEpoch = epoch
			if problem != "" {
				result.Status, result.Problem = "held", problem
			}
			return result, nil
		})
	switch {
	case errors.Is(err, errScopeAbsent):
		err = ErrInvalidDisable
	case errors.Is(err, errOperationConflict):
		err = ErrDisableConflict
	}
	return receipt, err
}

func LookupDisableReceipt(ctx context.Context, pool *pgxpool.Pool, scope, operation string) (DisableReceiptView, error) {
	if !validRef(scope) || !validRef(operation) {
		return DisableReceiptView{}, ErrInvalidDisable
	}
	receipt, _, err := readOperationReceipt[DisableReceipt](ctx, pool, disableOperationKey(scope, operation))
	if errors.Is(err, errOperationAbsent) {
		return DisableReceiptView{}, ErrDisableReceiptAbsent
	}
	if err != nil {
		return DisableReceiptView{}, err
	}
	view := DisableReceiptView{DisableReceipt: receipt}
	if receipt.Status == StatusEmployeeDisabled {
		ops := repository.NewSessionOperationRepo(pool)
		sessionOperation, err := ops.FindOperationByID(ctx, receipt.RunID)
		if err != nil {
			return DisableReceiptView{}, err
		}
		view.RevocationStatus, err = ops.OperationProjection(ctx, pool, sessionOperation.TargetUserID, receipt.SessionEpoch)
		if err != nil {
			return DisableReceiptView{}, err
		}
	}
	return view, nil
}

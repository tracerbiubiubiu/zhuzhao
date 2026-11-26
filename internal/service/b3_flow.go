package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3al"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/taskrunner"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const b3Submitter = "service:zhuzhao-b3"

type B3TaskTransport interface {
	Submit(context.Context, taskrunner.SubmitRequest) (*taskrunner.SubmitResponse, error)
	GetTask(context.Context, string) (json.RawMessage, error)
	RetryTask(context.Context, string, string) (json.RawMessage, error)
}

type b3ActiveList interface {
	Get(context.Context, b3al.Command) (b3al.ReadResult, error)
	Update(context.Context, b3al.Command) (b3al.UpdateResult, error)
	LookupUpdate(context.Context, b3al.Command) (b3al.UpdateResult, error)
}

type b3TicketAuthority func(context.Context, repository.B3Execution, string, func(context.Context) error) error

const b3CommandLifetime = time.Hour

type B3Flow struct {
	store        *repository.B3Store
	db           *pgxpool.Pool
	tr           B3TaskTransport
	al           b3ActiveList
	authorize    b3TicketAuthority
	policyModel  string
	subscription B3Subscription
}

func NewB3Flow(db *pgxpool.Pool, store *repository.B3Store, tr B3TaskTransport, al b3ActiveList, authorize b3TicketAuthority, policyModel string, subscription B3Subscription) *B3Flow {
	return &B3Flow{store: store, db: db, tr: tr, al: al, authorize: authorize, policyModel: policyModel, subscription: subscription}
}

// Handle processes one bounded R01 batch. Recovery queries the original task
// before any submit, including when a prior request committed without a reply.
func (f *B3Flow) Handle(ctx context.Context, _ json.RawMessage) error {
	for range 8 {
		delivery, err := f.store.Claim(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		call, cancel := context.WithTimeout(ctx, 20*time.Second)
		accepted := f.deliver(call, delivery)
		cancel()
		if err := f.store.DeliveryResult(ctx, delivery, accepted); err != nil {
			return err
		}
	}
	return nil
}

func (f *B3Flow) deliver(ctx context.Context, d repository.B3Delivery) bool {
	raw, err := f.tr.GetTask(ctx, d.TaskID)
	if b3ErrorCode(err, errcode.ErrNotFound) && !d.Accepted {
		params, _ := json.Marshal(b3protocol.TaskParams{OperationID: d.OperationID})
		response, err := f.tr.Submit(ctx, taskrunner.SubmitRequest{
			TaskID: d.TaskID, Action: d.Action, Params: params, SubmittedBy: b3Submitter, TimeoutSecs: 15,
		})
		return err == nil && response != nil && response.TaskID == d.TaskID
	}
	if err != nil {
		return false
	}
	var view struct {
		TaskID      string `json:"task_id"`
		Action      string `json:"action"`
		SubmittedBy string `json:"submitted_by"`
		Params      string `json:"params"`
		Status      string `json:"status"`
	}
	var params b3protocol.TaskParams
	if json.Unmarshal(raw, &view) != nil || view.TaskID != d.TaskID || view.Action != d.Action ||
		view.SubmittedBy != b3Submitter || b3protocol.DecodeBytes([]byte(view.Params), &params) != nil ||
		params.OperationID != d.OperationID {
		return false
	}
	switch view.Status {
	case "failed", "dead":
		f.recoverDelivery(ctx, d)
	case "canceled":
		f.finishUndelivered(ctx, d, "task_delivery_canceled")
	}
	return true
}

// A terminal queue delivery does not establish a terminal business outcome.
func (f *B3Flow) recoverDelivery(ctx context.Context, delivery repository.B3Delivery) {
	if time.Since(delivery.CreatedAt) >= b3CommandLifetime && !delivery.RemoteStarted {
		f.finishUndelivered(ctx, delivery, "execution_expired")
		return
	}
	if !delivery.RemoteStarted {
		// The worker only runs after its current claim. A retry of this
		// confirmed failed delivery reuses the original Taskrunner identity.
		_, _ = f.tr.RetryTask(ctx, delivery.TaskID, b3Submitter)
		return
	}
	run, err := f.store.Execution(ctx, delivery.TaskID)
	if err != nil || run.Status != "pending" || run.Action != b3protocol.Update {
		return
	}
	if err = f.store.ClaimExecution(ctx, &run); err != nil {
		return
	}
	defer f.store.ReleaseExecution(ctx, run)
	command, input, err := f.documentUpdateCommand(run)
	if err != nil {
		_ = f.unknownUpdate(ctx, run)
		return
	}
	err = f.authorize(ctx, run, f.policyModel, func(ctx context.Context) error {
		return f.store.AuthorizeDocumentUpdate(ctx, run.ActorID, input)
	})
	if err != nil {
		_ = f.unknownUpdate(ctx, run)
		return
	}
	result, err := f.al.LookupUpdate(ctx, command)
	if err != nil {
		_ = f.unknownUpdate(ctx, run)
		return
	}
	_ = f.finish(ctx, run, "succeeded", "", result)
}

func (f *B3Flow) finishUndelivered(ctx context.Context, delivery repository.B3Delivery, reason string) {
	if delivery.RemoteStarted {
		return
	}
	run, err := f.store.Execution(ctx, delivery.TaskID)
	if err != nil || run.Status != "pending" || f.store.ClaimExecution(ctx, &run) != nil {
		return
	}
	defer f.store.ReleaseExecution(ctx, run)
	if !run.RemoteStarted {
		_ = f.finish(ctx, run, "failed_no_effect", reason, nil)
	}
}

type B3Callback struct {
	ProtocolVersion int
	ActionVersion   int
	TaskID          string
	Action          string
	OperationID     string
	DeliveryID      string
	Params          json.RawMessage
}

type B3Receipt struct {
	TaskID      string          `json:"task_id"`
	OperationID string          `json:"operation_id"`
	State       string          `json:"state"`
	ReceiptID   string          `json:"receipt_id"`
	Result      json.RawMessage `json:"result,omitempty"`
}

func (f *B3Flow) Callback(ctx context.Context, input B3Callback) (B3Receipt, error) {
	var empty B3Receipt
	var params b3protocol.TaskParams
	if input.ProtocolVersion != 2 || input.ActionVersion != 1 ||
		!b3protocol.IsAction(input.Action) || !b3protocol.ValidID(input.OperationID) ||
		!b3protocol.ValidID(input.DeliveryID) || b3protocol.TaskID(input.OperationID) != input.TaskID ||
		b3protocol.DecodeBytes(input.Params, &params) != nil || params.OperationID != input.OperationID {
		return empty, errcode.ErrInvalidParams
	}
	run, err := f.store.Execution(ctx, input.TaskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return empty, errcode.ErrNotFound
	}
	if err != nil {
		return empty, errcode.ErrServiceUnavailable
	}
	if run.Action != input.Action || run.OperationID != input.OperationID {
		return empty, errcode.ErrConflict
	}
	if run.Status != "pending" {
		return terminalB3Receipt(run)
	}
	err = f.store.ClaimExecution(ctx, &run)
	if err != nil {
		return empty, errcode.ErrServiceUnavailable
	}
	// Retain the timestamp lease after request cancellation or a process stop.
	// A retry can recover an already committed ActiveList effect by its operation.
	defer func() {
		if ctx.Err() == nil {
			_ = f.store.ReleaseExecution(ctx, run)
		}
	}()
	switch run.Action {
	case b3protocol.Report:
		err = f.runReport(ctx, run)
	case b3protocol.Update:
		err = f.runUpdate(ctx, run)
	case b3protocol.Read:
		err = f.runRead(ctx, run)
	}
	if err != nil {
		return empty, err
	}
	run, err = f.store.Execution(ctx, input.TaskID)
	if err != nil {
		return empty, errcode.ErrServiceUnavailable
	}
	return terminalB3Receipt(run)
}

func terminalB3Receipt(run repository.B3Execution) (B3Receipt, error) {
	switch run.Status {
	case "succeeded":
		return B3Receipt{TaskID: run.TaskID, OperationID: run.OperationID, State: run.Status, ReceiptID: b3protocol.Identity("receipt_", run.OperationID)}, nil
	case "held":
		return B3Receipt{}, errcode.ErrForbidden
	case "failed_no_effect":
		return B3Receipt{}, errcode.ErrConflict
	default:
		return B3Receipt{}, errcode.ErrServiceUnavailable
	}
}

func b3ErrorCode(err error, sentinel *errcode.Error) bool {
	var business *errcode.Error
	return errors.As(err, &business) && business.Code == sentinel.Code
}

func deniedB3(err error) bool {
	return errors.Is(err, errcode.ErrForbidden) || errors.Is(err, errcode.ErrNoPermission) ||
		errors.Is(err, errcode.ErrNoRoles) || errors.Is(err, errcode.ErrNotFound) || errors.Is(err, errcode.ErrTicketNotFound)
}

func (f *B3Flow) finish(ctx context.Context, run repository.B3Execution, status, reason string, result any) error {
	var raw json.RawMessage
	if result != nil {
		var err error
		raw, err = json.Marshal(result)
		if err != nil {
			return errcode.ErrServiceUnavailable
		}
	}
	changed, err := f.store.Complete(ctx, run, status, reason, raw)
	if err != nil || !changed {
		return errcode.ErrServiceUnavailable
	}
	return nil
}

func (f *B3Flow) runReport(ctx context.Context, run repository.B3Execution) error {
	if time.Since(run.CreatedAt) >= b3CommandLifetime {
		return f.finish(ctx, run, "failed_no_effect", "execution_expired", nil)
	}
	var report b3protocol.ReportInput
	if b3protocol.DecodeBytes(run.Payload, &report) != nil || report.Profile != b3protocol.ReportProfile {
		return f.finish(ctx, run, "failed_no_effect", "report_input_unavailable", nil)
	}
	err := f.authorize(ctx, run, f.policyModel, func(ctx context.Context) error {
		return f.finish(ctx, run, "succeeded", "", report)
	})
	if deniedB3(err) {
		return f.finish(ctx, run, "held", "ticket_authorization_revoked", nil)
	}
	return err
}

func (f *B3Flow) documentUpdateCommand(run repository.B3Execution) (b3al.Command, b3protocol.DocumentUpdate, error) {
	var input b3protocol.DocumentUpdate
	if err := b3protocol.DecodeBytes(run.Payload, &input); err != nil {
		return b3al.Command{}, input, err
	}
	command := f.command(run)
	command.SubjectType, command.SubjectID = "human", strconv.FormatInt(run.ActorID, 10)
	command.AuthorizationRef, command.AuthorizationRevision = input.GrantRef, input.GrantRevision
	command.TypeName, command.DocumentID, command.Generation = input.TypeName, input.DocumentID, input.Generation
	command.ExpectedRevision, command.Patch = input.ExpectedRevision, input.Patch
	command.Entrypoint, command.ExpectedDefinitionHash = "ticket", run.DefinitionHash
	return command, input, nil
}

func (f *B3Flow) runUpdate(ctx context.Context, run repository.B3Execution) error {
	command, input, err := f.documentUpdateCommand(run)
	if err != nil {
		return f.finish(ctx, run, "failed_no_effect", "invalid_document_update", nil)
	}
	check := func(markSent bool) error {
		return f.authorize(ctx, run, f.policyModel, func(ctx context.Context) error {
			if err := f.store.AuthorizeDocumentUpdate(ctx, run.ActorID, input); err != nil {
				return err
			}
			if markSent {
				return f.store.MarkRemoteStarted(ctx, run)
			}
			return nil
		})
	}
	if run.RemoteStarted {
		if err := check(false); err != nil {
			return f.unknownUpdate(ctx, run)
		}
		result, err := f.al.LookupUpdate(ctx, command)
		if err == nil {
			return f.finish(ctx, run, "succeeded", "", result)
		}
		var upstream *b3al.HTTPError
		if !errors.As(err, &upstream) || upstream.Status != 404 || !time.Now().Before(command.ExpiresAt) {
			return f.unknownUpdate(ctx, run)
		}
	}
	if !time.Now().Before(command.ExpiresAt) {
		return f.finish(ctx, run, "failed_no_effect", "execution_expired", nil)
	}
	if err := check(true); err != nil {
		if run.RemoteStarted {
			return f.unknownUpdate(ctx, run)
		}
		if deniedB3(err) {
			return f.finish(ctx, run, "held", "document_authorization_revoked", nil)
		}
		return errcode.ErrServiceUnavailable
	}
	result, err := f.al.Update(ctx, command)
	return f.finishAL(ctx, run, result, err)
}

func (f *B3Flow) unknownUpdate(ctx context.Context, run repository.B3Execution) error {
	if err := f.store.RecordPendingReason(ctx, run, "document_outcome_unconfirmed"); err != nil {
		return fmt.Errorf("record unconfirmed ActiveList operation: %w", err)
	}
	return errcode.ErrServiceUnavailable
}

func (f *B3Flow) command(run repository.B3Execution) b3al.Command {
	return b3al.Command{
		OperationID: run.OperationID, InvocationID: b3protocol.Identity("inv_", run.OperationID),
		RootID: run.OperationID, RootBudgetRef: b3protocol.Identity("budget_", run.OperationID),
		Fence:       strconv.FormatInt(run.ExecutionGeneration, 10),
		SubmittedAt: run.CreatedAt, ExpiresAt: run.CreatedAt.Add(b3CommandLifetime),
	}
}

func (f *B3Flow) finishAL(ctx context.Context, run repository.B3Execution, result any, err error) error {
	if err == nil {
		return f.finish(ctx, run, "succeeded", "", result)
	}
	var response *b3al.HTTPError
	switch {
	case errors.As(err, &response):
		switch response.Status {
		case 409:
			if !run.RemoteStarted {
				return f.finish(ctx, run, "failed_no_effect", "document_version_conflict", nil)
			}
		case 403, 404:
			if !run.RemoteStarted {
				return f.finish(ctx, run, "held", "document_unavailable_or_not_authorized", nil)
			}
		case 400, 422:
			if !run.RemoteStarted {
				return f.finish(ctx, run, "failed_no_effect", "document_input_rejected", nil)
			}
		}
	case errors.Is(err, b3al.ErrInvalid) && !run.RemoteStarted:
		return f.finish(ctx, run, "failed_no_effect", "document_input_invalid", nil)
	}
	if run.Action == b3protocol.Update {
		return f.unknownUpdate(ctx, run)
	}
	return errcode.ErrServiceUnavailable
}

func (f *B3Flow) Status(ctx context.Context, taskID, operationID string) (B3Receipt, error) {
	if !b3protocol.ValidID(operationID) || b3protocol.TaskID(operationID) != taskID {
		return B3Receipt{}, errcode.ErrInvalidParams
	}
	run, err := f.store.Execution(ctx, taskID)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && run.OperationID != operationID) {
		return B3Receipt{}, errcode.ErrNotFound
	}
	if err != nil {
		return B3Receipt{}, fmt.Errorf("read original B3 receipt: %w", err)
	}
	receipt, err := terminalB3Receipt(run)
	if err != nil || run.Action != b3protocol.Read {
		return receipt, err
	}
	var input b3protocol.SourceRead
	if b3protocol.DecodeBytes(run.Payload, &input) != nil {
		return B3Receipt{}, errcode.ErrServiceUnavailable
	}
	grant, err := f.store.Grant(ctx, input.GrantRef)
	if err != nil {
		return B3Receipt{}, err
	}
	command := f.sourceReadCommand(run, input)
	if b3al.Authorize(command, grant, false) != nil {
		return B3Receipt{}, errcode.ErrNotFound
	}
	// Recheck AL's current row and field scope, then return the original
	// stored observation and its timestamp.
	command.ExpiresAt = time.Now().Add(25 * time.Second)
	if _, err := f.al.Get(ctx, command); err != nil {
		var upstream *b3al.HTTPError
		if errors.As(err, &upstream) && (upstream.Status == 403 || upstream.Status == 404) {
			return B3Receipt{}, errcode.ErrNotFound
		}
		return B3Receipt{}, errcode.ErrServiceUnavailable
	}
	receipt.Result = run.Result
	return receipt, nil
}

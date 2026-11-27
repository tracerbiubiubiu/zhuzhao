package service

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"time"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3al"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// B3Subscription is the single fixed rule. Changing its inputs requires a new
// admitted profile and a controlled cutover.
type B3Subscription struct {
	TypeName      string
	Generation    string
	ConsumerID    string
	RuleID        string
	RuleVersion   string
	GrantRef      string
	GrantRevision string
	SubjectID     string
	Fields        []string
	EventFields   []string
}

func (f *B3Flow) Push(ctx context.Context, input b3protocol.SourceDelivery) (b3protocol.SourceAck, error) {
	var empty b3protocol.SourceAck
	var event b3protocol.SourceEvent
	s := f.subscription
	if input.SchemaVersion != "1" || !b3protocol.ValidID(input.DeliveryID) ||
		input.ConsumerID != s.ConsumerID || b3protocol.DecodeBytes(input.Event, &event) != nil ||
		b3protocol.ValidateSource(event, s.TypeName, s.Generation) != nil {
		return empty, errcode.ErrInvalidParams
	}
	fields := s.EventFields
	if len(fields) == 0 {
		fields = s.Fields
	}
	for field := range event.Data.Values {
		if !slices.Contains(fields, field) {
			return empty, errcode.ErrInvalidParams
		}
	}
	hash, err := b3protocol.SourceHash(input.Event)
	if err != nil {
		return empty, errcode.ErrInvalidParams
	}
	stage, err := f.stageSource(ctx, event, hash, input.Event)
	if errors.Is(err, errcode.ErrConflict) {
		return empty, err
	}
	if err != nil {
		return empty, errcode.ErrServiceUnavailable
	}
	return b3protocol.SourceAck{
		SchemaVersion: "1", DeliveryID: input.DeliveryID, Source: event.Source, EventID: event.ID,
		PayloadSHA256: hash, StageCommitRef: stage, State: "durably_staged",
	}, nil
}

func (f *B3Flow) stageSource(ctx context.Context, event b3protocol.SourceEvent, hash string, raw json.RawMessage) (string, error) {
	if f.draining {
		return f.store.ExistingSourceStage(ctx, event.Source, event.ID, hash)
	}
	s := f.subscription
	scope := event.Source + "/" + event.ID
	var derived *repository.B3Execution
	if event.CanDerive(s.RuleID) {
		payload, err := json.Marshal(b3protocol.SourceRead{
			TypeName: s.TypeName, Generation: s.Generation, DocumentID: event.Data.DocumentID,
			Fields: s.Fields, EventID: event.ID, EventRevision: event.Data.DocumentRevision, Causality: event.Data.Causality,
			GrantRef: s.GrantRef, GrantRevision: s.GrantRevision, SubjectID: s.SubjectID, RuleID: s.RuleID,
		})
		if err != nil {
			return "", err
		}
		operation := b3protocol.Identity("op_evt_", scope+"/"+s.RuleID+"/"+s.RuleVersion+"/0")
		derived = &repository.B3Execution{
			TicketTaskRun: model.TicketTaskRun{OperationID: operation, TaskID: b3protocol.TaskID(operation), Action: b3protocol.Read},
			Payload:       payload,
		}
	}
	cursor, _ := strconv.ParseInt(event.Data.Cursor, 10, 64)
	return f.store.StageSource(ctx, event.Source, event.ID, event.Data.StreamID, cursor, hash,
		b3protocol.Identity("stage_", scope), raw, derived)
}

func (f *B3Flow) runRead(ctx context.Context, run repository.B3Execution) error {
	if time.Since(run.CreatedAt) >= b3CommandLifetime {
		return f.finish(ctx, run, "failed_no_effect", "execution_expired", nil)
	}
	var input b3protocol.SourceRead
	if b3protocol.DecodeBytes(run.Payload, &input) != nil {
		return f.finish(ctx, run, "failed_no_effect", "invalid_document_observation", nil)
	}
	command := f.sourceReadCommand(run, input)
	grant, err := f.store.Grant(ctx, input.GrantRef)
	if deniedB3(err) || (err == nil && b3al.Authorize(command, grant, false) != nil) {
		return f.finish(ctx, run, "held", "subscription_authorization_revoked", nil)
	}
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	result, err := f.al.Get(ctx, command)
	return f.finishAL(ctx, run, result, err)
}

func (f *B3Flow) sourceReadCommand(run repository.B3Execution, input b3protocol.SourceRead) b3al.Command {
	command := f.command(run)
	command.SubjectType, command.SubjectID = "service", input.SubjectID
	command.AuthorizationRef, command.AuthorizationRevision = input.GrantRef, input.GrantRevision
	command.TypeName, command.DocumentID, command.Generation = input.TypeName, input.DocumentID, input.Generation
	command.Fields, command.SourceEventID, command.SourceEventRevision = input.Fields, input.EventID, input.EventRevision
	command.ParentEventID, command.Entrypoint = input.EventID, "subscription"
	command.RootID, command.RootBudgetRef = input.Causality.RootID, input.Causality.BudgetRef
	command.Depth, command.RulePath = input.Causality.Depth+1, append(input.Causality.RulePath, input.RuleID)
	return command
}

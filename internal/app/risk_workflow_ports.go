package app

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/app/riskworkflow"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/riskremediation"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

type riskWorkflowPorts struct {
	*riskworkflow.ReceiptService
	pool       *pgxpool.Pool
	tickets    *ticket.Service
	storage    *pkgstorage.ConfigStore
	enabled    bool
	assignment *riskworkflow.AssignmentService
}

func (s riskWorkflowPorts) Current(ctx context.Context, actor, ticketID int64) (model.RiskWorkflowContext, error) {
	current, err := s.ReceiptService.Current(ctx, actor, ticketID)
	if err != nil {
		return model.RiskWorkflowContext{}, err
	}
	current.WritesEnabled = s.canWrite()
	return current, nil
}

func (s riskWorkflowPorts) canWrite() bool {
	if !s.enabled || s.storage == nil {
		return false
	}
	cfg := s.storage.Load()
	return cfg.Enabled && cfg.Bucket != ""
}

func (s riskWorkflowPorts) writes() (*riskworkflow.Service, error) {
	if !s.enabled || s.storage == nil {
		return nil, errcode.ErrServiceUnavailable
	}
	cfg := s.storage.Load()
	if cfg == nil || !cfg.Enabled || cfg.Bucket == "" {
		return nil, errcode.ErrServiceUnavailable
	}
	evidence := storageservice.NewBoundTicketEvidenceValidator(s.pool, cfg.Bucket)
	return riskworkflow.NewService(s.pool, s.tickets, evidence), nil
}

func (s riskWorkflowPorts) Finish(ctx context.Context, cmd riskremediation.FinishCommand, ticketInput ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	service, err := s.writes()
	if err != nil {
		return riskremediation.Result{}, err
	}
	return service.Finish(ctx, riskworkflow.FinishCommand{Command: cmd.Command, Items: cmd.Items, TicketInput: ticketInput})
}

func (s riskWorkflowPorts) Review(ctx context.Context, cmd riskremediation.ReviewCommand, ticketInput ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	service, err := s.writes()
	if err != nil {
		return riskremediation.Result{}, err
	}
	return service.Review(ctx, riskworkflow.ReviewCommand{Command: cmd.Command, Items: cmd.Items, TicketInput: ticketInput})
}

func (s riskWorkflowPorts) RequestAcceptance(ctx context.Context, cmd riskremediation.AcceptanceRequest, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	service, err := s.writes()
	if err != nil {
		return riskremediation.Result{}, err
	}
	return service.RequestAcceptance(ctx, riskworkflow.AcceptanceRequestCommand{AcceptanceRequest: cmd, TicketInput: input})
}

func (s riskWorkflowPorts) DecideAcceptance(ctx context.Context, cmd riskremediation.AcceptanceDecision, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	service, err := s.writes()
	if err != nil {
		return riskremediation.Result{}, err
	}
	return service.DecideAcceptance(ctx, riskworkflow.AcceptanceDecisionCommand{AcceptanceDecision: cmd, TicketInput: input})
}

func (s riskWorkflowPorts) Handoff(ctx context.Context, cmd riskremediation.HandoffCommand) (riskremediation.Result, error) {
	service, err := s.writes()
	if err != nil {
		return riskremediation.Result{}, err
	}
	return service.Handoff(ctx, cmd)
}

func (s riskWorkflowPorts) ReworkAcceptance(ctx context.Context, cmd riskremediation.AcceptanceRework, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	service, err := s.writes()
	if err != nil {
		return riskremediation.Result{}, err
	}
	return service.ReworkAcceptance(ctx, cmd, input)
}

func (s riskWorkflowPorts) AssignHandler(ctx context.Context, cmd riskremediation.Command, assignee int64, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	if !s.enabled || s.assignment == nil {
		return riskremediation.Result{}, errcode.ErrServiceUnavailable
	}
	return s.assignment.AssignHandler(ctx, riskworkflow.AssignHandlerCommand{
		AssignmentCommand: riskAssignmentCommand(cmd, input), AssignedToUserID: assignee,
	})
}

func (s riskWorkflowPorts) AcceptAssignment(ctx context.Context, cmd riskremediation.Command, input ticket.RiskWorkflowTicketInput) (riskremediation.Result, error) {
	if !s.enabled || s.assignment == nil {
		return riskremediation.Result{}, errcode.ErrServiceUnavailable
	}
	return s.assignment.AcceptAssignment(ctx, riskAssignmentCommand(cmd, input))
}

func riskAssignmentCommand(cmd riskremediation.Command, input ticket.RiskWorkflowTicketInput) riskworkflow.AssignmentCommand {
	return riskworkflow.AssignmentCommand{
		ActorID: cmd.ActorID, OperationID: cmd.OperationID, TicketID: cmd.TicketID,
		ExpectedTicketRevision: cmd.ExpectedTicketRevision, TicketInput: input,
	}
}

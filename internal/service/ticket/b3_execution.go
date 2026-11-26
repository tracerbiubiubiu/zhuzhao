package ticket

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3protocol"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func (s *Service) SetB3Store(store *repository.B3Store, dispatchEnabled bool) {
	s.b3 = store
	s.b3DispatchEnabled = dispatchEnabled
}

func b3Action(ref actionRef) string {
	action, _ := ref.Params["action"].(string)
	if ref.Name == "spawn_task" && b3protocol.IsAction(action) {
		return action
	}
	return ""
}

func hasB3Actions(meta *TransitionMeta) bool {
	for _, ref := range parsePostActions(meta) {
		if b3Action(ref) != "" {
			return true
		}
	}
	return false
}

func (s *Service) stageB3Actions(ctx context.Context, tx pgx.Tx, meta *TransitionMeta, ticket *model.Ticket, cmd evidenceTransitionRequest, actor int64) error {
	for i, ref := range parsePostActions(meta) {
		action := b3Action(ref)
		if action == "" {
			continue
		}
		purpose, fromHTTP := middleware.IAMRequestFromContext(ctx)
		if !s.b3DispatchEnabled || s.b3 == nil || ticket.DefinitionRef == nil || !fromHTTP || purpose.UserID != actor {
			return errcode.ErrNoPermission
		}
		params, err := json.Marshal(ref.Params["params"])
		if err != nil {
			return errcode.ErrInvalidParams
		}
		switch action {
		case b3protocol.Report:
			var input b3protocol.ReportInput
			// The persisted snapshot is server-derived. The published action
			// accepts only a known report profile.
			var profile struct {
				Name string `json:"report_profile"`
			}
			if b3protocol.DecodeBytes(params, &profile) != nil || profile.Name != b3protocol.ReportProfile {
				return errcode.ErrInvalidParams
			}
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(cmd.patch, &fields); err != nil {
				return errcode.ErrInvalidParams
			}
			input = b3protocol.ReportInput{Profile: profile.Name, FieldsCount: len(fields), HasComment: cmd.comment != ""}
			params, err = json.Marshal(input)
		case b3protocol.Update:
			var input b3protocol.DocumentUpdate
			if b3protocol.DecodeBytes(params, &input) != nil || len(input.Patch) == 0 {
				return errcode.ErrInvalidParams
			}
			if err := s.b3.AuthorizeDocumentUpdate(ctx, actor, input); err != nil {
				return err
			}
		default:
			return errcode.ErrNoPermission
		}
		if err != nil {
			return err
		}
		operation := b3protocol.Identity("op_ticket_", fmt.Sprintf("%d/%d/%d", ticket.ID, ticket.Revision, i))
		run := repository.B3Execution{
			TicketTaskRun: model.TicketTaskRun{
				OperationID: operation, TaskID: b3protocol.TaskID(operation), Action: action,
				TicketRevision: ticket.Revision, DefinitionHash: ticket.DefinitionRef.DefinitionHash, Node: ticket.Status,
			},
			TicketID: ticket.ID, ActorID: actor, ActionIndex: i, Payload: params, TransitionRoles: meta.Roles,
		}
		if err := s.b3.Stage(ctx, tx, run); err != nil {
			return fmt.Errorf("stage ticket task: %w", err)
		}
	}
	return nil
}

// RunB3Authorized holds the IAM guard until any local outcome is committed. A
// remote effect must execute after this guard has been released.
func (s *Service) RunB3Authorized(ctx context.Context, execution repository.B3Execution, modelText string, run func(context.Context) error) error {
	return repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		actor, id := execution.ActorID, execution.TicketID
		roles, err := s.freshCommandRoles(ctx, actor, modelText, ticketPolicyRoute{path: "/api/v1/tickets/transition", method: "POST"})
		if err != nil {
			return err
		}
		ctx = context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
		ticket, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.authorizeCheck(ctx, actor, "transition", strconv.FormatInt(id, 10)); err != nil {
			return err
		}
		// The ticket may have advanced. Evaluate the original edge condition
		// against current ownership, assignment and IAM membership.
		var orgAdmin bool
		if slices.Contains(execution.TransitionRoles, "org_admin") {
			orgAdmin, err = s.delegation.IsOrgAdminOrOwner(ctx, actor, ticket.OrgID)
			if err != nil {
				return err
			}
		}
		if !checkCondition(&TransitionMeta{Roles: execution.TransitionRoles}, resolveActor(ticket, actor, roles, orgAdmin)) {
			return errcode.ErrNoPermission
		}
		return run(ctx)
	})
}

package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/b3al"
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

// parseB3TicketParams shares the frozen contract between publication and
// staging. For reports only the profile is configured. The result fields
// are derived from the accepted transition when it runs.
func parseB3TicketParams(action string, value any) (*b3protocol.DocumentUpdate, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, errors.New("任务参数无法编码")
	}
	switch action {
	case b3protocol.Report:
		var profile struct {
			Name string `json:"report_profile"`
		}
		if b3protocol.DecodeBytes(raw, &profile) != nil || profile.Name != b3protocol.ReportProfile {
			return nil, errors.New("报告动作仅接受固定的 report_profile")
		}
		return nil, nil
	case b3protocol.Update:
		var update b3protocol.DocumentUpdate
		if b3protocol.DecodeBytes(raw, &update) != nil || b3al.ValidateDocumentUpdate(update) != nil {
			return nil, errors.New("清单更新需要有效的目标、版本、授权引用及标量 patch")
		}
		return &update, nil
	default:
		return nil, errors.New("清单读取动作仅可由清单变更订阅发起")
	}
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
		if action == b3protocol.Read {
			return errcode.ErrNoPermission
		}
		update, err := parseB3TicketParams(action, ref.Params["params"])
		if err != nil {
			return errcode.ErrInvalidParams
		}
		var payload any
		switch action {
		case b3protocol.Report:
			var fields map[string]json.RawMessage
			if err := json.Unmarshal(cmd.patch, &fields); err != nil {
				return errcode.ErrInvalidParams
			}
			payload = b3protocol.ReportInput{Profile: b3protocol.ReportProfile, FieldsCount: len(fields), HasComment: cmd.comment != ""}
		case b3protocol.Update:
			if err := s.b3.AuthorizeDocumentUpdate(ctx, actor, *update); err != nil {
				return err
			}
			payload = update
		}
		params, err := json.Marshal(payload)
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

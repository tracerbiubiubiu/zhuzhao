package ticket

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// ListAvailableActions returns the input contract of the pinned rule set.
func (s *Service) ListAvailableActions(ctx context.Context, id, actorID int64) (*model.TicketActionList, error) {
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok {
		return s.listAvailableActions(ctx, id, actorID, nil, nil)
	}
	var result *model.TicketActionList
	err := repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		if purpose.UserID != actorID {
			return errcode.ErrNoPermission
		}
		if err := s.freshCommandCredential(ctx, actorID, purpose); err != nil {
			return err
		}
		roles, policies, err := repository.NewUserRepo(s.db).CurrentIAMAuthorization(ctx, actorID)
		if err != nil {
			return err
		}
		ctx = context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actorID, roles: roles})
		grants, err := commandRouteGrants(purpose.Model, roles, policies, []ticketPolicyRoute{{"/api/v1/tickets/transfer", "POST"}, {"/api/v1/tickets/withdraw", "POST"}})
		if err != nil {
			return err
		}
		if err := s.authorizeCheck(ctx, actorID, "read", strconv.FormatInt(id, 10)); err != nil {
			return err
		}
		// Pin the capability coordinates through the read; all writers lock the same ticket.
		if err := s.lockCommandTickets(ctx, tx, id); err != nil {
			return err
		}
		result, err = s.listAvailableActions(ctx, id, actorID, tx, grants)
		return err
	})
	return result, err
}

func (s *Service) listAvailableActions(ctx context.Context, id, actorID int64, tx pgx.Tx, grants []bool) (*model.TicketActionList, error) {
	result := &model.TicketActionList{
		Actions: make([]model.TicketAvailableAction, 0),
		Fields:  make([]model.PublishedTicketFormField, 0),
	}
	record, err := s.Get(ctx, id, actorID)
	if err != nil {
		return nil, err
	}
	result.TicketID, result.Revision = record.ID, record.Revision
	if record.DefinitionResolution == "degraded" {
		return result, nil
	}
	ttype, fields, ref, err := s.resolveTicketDefinition(ctx, record)
	if err != nil {
		return nil, err
	}
	result.Capabilities, err = s.handlingCapabilities(ctx, tx, record, ttype, actorID, grants)
	if err != nil {
		return nil, err
	}
	for _, f := range fields {
		field, err := projectPublishedTicketField(record.TypeCode, f)
		if err != nil {
			return nil, err
		}
		result.Fields = append(result.Fields, field)
	}
	if err := s.authorizeCheck(ctx, actorID, "transition", strconv.FormatInt(id, 10)); err != nil {
		if errors.Is(err, errcode.ErrNoPermission) {
			return result, nil
		}
		return nil, err
	}
	roles, err := s.getRoles(ctx, actorID)
	if err != nil {
		return nil, err
	}
	orgAdmin, _ := s.delegation.IsOrgAdminOrOwner(ctx, actorID, record.OrgID)
	ctx = withDictQuerier(ctx, repository.IAMDB(ctx, s.db))
	result.Actions, err = availableTicketActionContracts(ctx, record, ttype, ref, resolveActor(record, actorID, roles, orgAdmin))
	if err != nil {
		return nil, err
	}
	return result, nil
}

func availableTicketActionContracts(ctx context.Context, ticket *model.Ticket, ttype *model.TicketType, ref *model.TicketDefinitionRef, actor transitionActor) ([]model.TicketAvailableAction, error) {
	var transitions map[string][]string
	if json.Unmarshal(ttype.Transitions, &transitions) != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	actions := make([]model.TicketAvailableAction, 0, len(transitions[ticket.Status]))
	for _, to := range transitions[ticket.Status] {
		if reservedHandlingEdge(ticket.Status, to) {
			continue
		}
		tm, err := parseTransitionMeta(ttype.TransitionMeta, ticket.Status, to)
		if err != nil {
			return nil, errcode.ErrServiceUnavailable
		}
		if !checkCondition(tm, actor) {
			continue
		}
		collect, err := transitionCollectFields(ttype.TransitionMeta, ticket.Status, to)
		if err != nil {
			return nil, err
		}
		// Validators for collecting actions see the candidate values at submit.
		if len(collect) == 0 && checkValidator(ctx, tm, ticket) != nil {
			continue
		}
		action := model.TicketAvailableAction{
			ActionID: transitionKey(ticket.Status, to), To: to, Label: to,
			ExpectedRevision: ticket.Revision, DefinitionRef: ref,
			CollectFields: make([]model.TicketActionCollectField, 0, len(collect)),
		}
		if tm != nil {
			action.CommentRequired = tm.CommentRequired
			if tm.Label != "" {
				action.Label = tm.Label
			}
		}
		for _, field := range collect {
			action.CollectFields = append(action.CollectFields, model.TicketActionCollectField{
				Key: field.Key, Required: field.Required,
			})
		}
		actions = append(actions, action)
	}
	return actions, nil
}

func projectPublishedTicketField(code string, f *model.TicketTypeField) (model.PublishedTicketFormField, error) {
	if f == nil || f.TypeCode != code || f.FieldID <= 0 {
		return model.PublishedTicketFormField{}, errcode.ErrServiceUnavailable
	}
	field := model.PublishedTicketFormField{
		TypeCode: code, FieldID: f.FieldID, FieldKey: f.FieldKey, FieldLabel: f.FieldLabel,
		FieldType: f.FieldType, FieldOptions: f.FieldOptions, Required: f.Required,
		ValidateRegex: f.ValidateRegex, SortOrder: f.SortOrder,
	}
	if len(f.FieldMeta) != 0 && json.Unmarshal(f.FieldMeta, &field.FieldMeta) != nil {
		return model.PublishedTicketFormField{}, errcode.ErrServiceUnavailable
	}
	return field, nil
}

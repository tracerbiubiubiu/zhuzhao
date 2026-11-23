package ticket

import (
	"encoding/json"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// ValidateRiskDispatchDefinition checks the fixed remediation profile before
// creating a ticket. Current authorization and evidence are checked at execution.
// It does not publish, alter or grant permission to use the supplied definition.
func ValidateRiskDispatchDefinition(typ *model.TicketType, fields []*model.TicketTypeField) error {
	return validateRiskWorkflowDefinition(typ, fields, true)
}

func validateRiskWorkflowDefinition(typ *model.TicketType, fields []*model.TicketTypeField, checkAssignment bool) error {
	if typ == nil || validateTypeGraph(typ.States, typ.Transitions) != nil {
		return errcode.ErrServiceUnavailable
	}
	var states []string
	var graph map[string][]string
	if json.Unmarshal(typ.States, &states) != nil || json.Unmarshal(typ.Transitions, &graph) != nil ||
		validateTransitionMeta(states, typ.TransitionMeta) != nil || validateTransitionEdges(states, graph, typ.TransitionMeta, nil) != nil {
		return errcode.ErrServiceUnavailable
	}
	byKey := make(map[string]*model.TicketTypeField, len(fields))
	for _, field := range fields {
		if field == nil || byKey[field.FieldKey] != nil {
			return errcode.ErrServiceUnavailable
		}
		byKey[field.FieldKey] = field
	}
	if validatePublishedEvidenceSpec(typ.TransitionMeta, fields) != nil {
		return errcode.ErrServiceUnavailable
	}
	sm, err := FromTicketType(typ)
	if err != nil {
		return errcode.ErrServiceUnavailable
	}
	edges := [][2]string{
		{StatusInProgress, StatusPendingVerify}, {StatusPendingVerify, StatusPendingVerify},
		{StatusPendingVerify, StatusClosed}, {StatusPendingVerify, StatusInProgress},
	}
	if checkAssignment {
		edges = append(edges, [2]string{StatusOpen, StatusAssigned}, [2]string{StatusAssigned, StatusInProgress})
	}
	for _, edge := range edges {
		if !sm.CanTransition(edge[0], edge[1]) {
			return errcode.ErrServiceUnavailable
		}
		meta, err := parseTransitionMeta(typ.TransitionMeta, edge[0], edge[1])
		if err != nil || (meta != nil && len(meta.PostActions) > 0) {
			return errcode.ErrServiceUnavailable
		}
		collect, err := transitionCollectFields(typ.TransitionMeta, edge[0], edge[1])
		if err != nil {
			return errcode.ErrServiceUnavailable
		}
		switch edge[0] {
		case StatusOpen:
			// The trusted initial bridge has no assignee, comment or evidence
			// input. Actual dispatcher roles/validator are checked before INSERT.
			if len(collect) > 0 || (meta != nil && meta.CommentRequired) ||
				!checkCondition(meta, transitionActor{IsCreator: true, IsOrgAdmin: true, IsAdmin: true}) {
				return errcode.ErrServiceUnavailable
			}
		case StatusAssigned, StatusInProgress:
			// A separately assigned individual must be able to accept and
			// submit without also being the creator or a global/org admin.
			if !checkCondition(meta, transitionActor{IsAssignee: true}) {
				return errcode.ErrServiceUnavailable
			}
		case StatusPendingVerify:
			// Dispatch creates the ticket as the risk owner. That owner can
			// retest after assigning a different individual to do the repair.
			if !checkCondition(meta, transitionActor{IsCreator: true}) {
				return errcode.ErrServiceUnavailable
			}
		}
		for _, item := range collect {
			field := byKey[item.Key]
			if field == nil || field.FieldType == "file" || field.FieldType == "richtext" {
				return errcode.ErrServiceUnavailable
			}
		}
	}
	return nil
}

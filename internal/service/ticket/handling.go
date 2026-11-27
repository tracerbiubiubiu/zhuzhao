package ticket

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

const (
	transferAction = "transfer"
	withdrawAction = "withdraw"
)

func reservedHandlingEdge(from, to string) bool {
	return from == StatusCancelled || to == StatusCancelled
}

func withdrawalRule(raw json.RawMessage, edges map[string][]string) (*TransitionMeta, error) {
	if !slices.Contains(edges[StatusOpen], StatusCancelled) || len(edges[StatusCancelled]) != 0 {
		return nil, nil
	}
	for from, targets := range edges {
		if from != StatusOpen && slices.Contains(targets, StatusCancelled) {
			return nil, nil
		}
	}
	meta, err := parseTransitionMeta(raw, StatusOpen, StatusCancelled)
	if err != nil {
		return nil, err
	}
	if meta == nil || !slices.Equal(meta.Roles, []string{"creator"}) || len(meta.PostActions) != 0 {
		return nil, nil
	}
	fields, err := transitionCollectFields(raw, StatusOpen, StatusCancelled)
	if err != nil || len(fields) > 0 {
		return nil, err
	}
	return meta, nil
}

// Both discovery and execution evaluate the bound graph and the same L3
// relationship. The enclosing command separately checks L1 and active risk links.
func (s *Service) handlingAllowed(ctx context.Context, action string, t *model.Ticket, typ *model.TicketType, actor int64) (*TransitionMeta, error) {
	if err := s.authorizeCheck(ctx, actor, action, strconv.FormatInt(t.ID, 10)); err != nil {
		return nil, err
	}
	var edges map[string][]string
	if err := json.Unmarshal(typ.Transitions, &edges); err != nil {
		return nil, errcode.ErrServiceUnavailable
	}
	if action == transferAction {
		if t.OrgID <= 0 || t.AssignedTo == nil || t.Status == StatusClosed || t.Status == StatusRejected || t.Status == StatusCancelled || len(edges[t.Status]) == 0 {
			return nil, errcode.ErrConcurrentModification
		}
		return nil, nil
	}
	if actor != t.CreatedBy {
		return nil, errcode.ErrNoPermission
	}
	if !t.WithdrawalEligible || t.Status != StatusOpen || t.AssignedTo != nil {
		return nil, errcode.ErrConcurrentModification
	}
	rule, err := withdrawalRule(typ.TransitionMeta, edges)
	if err != nil {
		return nil, err
	}
	if rule == nil {
		return nil, errcode.ErrNoPermission
	}
	return rule, checkValidator(withDictQuerier(ctx, repository.IAMDB(ctx, s.db)), rule, t)
}

// The GET exposes grants only for the authenticated HTTP actor. A direct
// internal reader without an HTTP credential has no static route capability.
func (s *Service) handlingCapabilities(ctx context.Context, tx pgx.Tx, row *model.Ticket, typ *model.TicketType, actor int64, grants []bool) (*model.TicketHandlingCapabilities, error) {
	if tx == nil {
		return nil, nil
	}
	caps := &model.TicketHandlingCapabilities{}
	if !grants[0] && !grants[1] {
		return caps, nil
	}
	if err := s.requireOrdinaryTicketsTx(ctx, tx, row.ID); err != nil {
		if errors.Is(err, errcode.ErrNoPermission) {
			return caps, nil
		}
		return nil, err
	}
	for i, action := range []string{transferAction, withdrawAction} {
		if !grants[i] {
			continue
		}
		rule, err := s.handlingAllowed(ctx, action, row, typ, actor)
		if errors.Is(err, errcode.ErrNoPermission) || errors.Is(err, errcode.ErrConcurrentModification) || errors.Is(err, errcode.ErrInvalidParams) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if action == transferAction {
			caps.Transfer = true
		} else {
			caps.Withdraw = true
			caps.WithdrawCommentRequired = rule.CommentRequired
		}
	}
	return caps, nil
}

func (s *Service) transferCandidates(ctx context.Context, q model.TicketPeopleQuery, actor int64) ([]model.TicketPerson, error) {
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	if !ok || purpose.UserID != actor {
		return nil, errcode.ErrNoPermission
	}
	var result []model.TicketPerson
	err := repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.freshCommandCredential(ctx, actor, purpose); err != nil {
			return err
		}
		roles, err := s.freshCommandRoles(ctx, actor, purpose.Model, ticketPolicyRoute{"/api/v1/tickets/transfer", "POST"})
		if err != nil {
			return err
		}
		ctx = context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles})
		if err := s.authorizeCheck(ctx, actor, "read", strconv.FormatInt(q.TicketID, 10)); err != nil {
			return err
		}
		row, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, q.TicketID)
		if err != nil {
			return err
		}
		if err := s.requireOrdinaryTicketsTx(ctx, tx, row.ID); err != nil {
			return err
		}
		typ, _, _, err := s.resolveTicketDefinition(ctx, row)
		if err != nil {
			return err
		}
		if _, err := s.handlingAllowed(ctx, transferAction, row, typ, actor); err != nil {
			return err
		}
		result, err = repository.NewTicketPeopleRepo(s.db).SearchCandidates(ctx, q.Query, row.OrgID, *row.AssignedTo)
		return err
	})
	return result, err
}

func handlingID(raw string) (int64, error) {
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || n <= 0 || strconv.FormatInt(n, 10) != raw {
		return 0, errcode.ErrInvalidParams
	}
	return n, nil
}

func (s *Service) Transfer(ctx context.Context, req model.TicketTransferRequest, key string, actor int64) (*model.TicketHandlingResult, error) {
	target, err := handlingID(req.ToUserID)
	if err != nil {
		return nil, err
	}
	return s.handleTicket(ctx, req.TicketWithdrawRequest, transferAction, target, key, actor)
}

func (s *Service) Withdraw(ctx context.Context, req model.TicketWithdrawRequest, key string, actor int64) (*model.TicketHandlingResult, error) {
	return s.handleTicket(ctx, req, withdrawAction, 0, key, actor)
}

func (s *Service) handleTicket(ctx context.Context, req model.TicketWithdrawRequest, action string, target int64, key string, actor int64) (*model.TicketHandlingResult, error) {
	id, err := handlingID(req.ID)
	if err != nil {
		return nil, err
	}
	rev, err := handlingID(req.ExpectedRevision)
	if err != nil {
		return nil, err
	}
	if key == "" || len(key) > 128 || strings.IndexFunc(key, func(r rune) bool { return r < '!' || r > '~' }) >= 0 || len(req.Comment) > RichtextMaxBytes {
		return nil, errcode.ErrInvalidParams
	}
	if _, ok := middleware.IAMRequestFromContext(ctx); !ok {
		return nil, errcode.ErrNoPermission
	}
	raw, err := json.Marshal(struct {
		Action  string
		Target  int64
		Request model.TicketWithdrawRequest
	}{action, target, req})
	if err != nil {
		return nil, err
	}
	digest := sha256.Sum256(raw)
	var result *model.TicketHandlingResult
	err = s.withHTTPCommand(ctx, action, actor, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.authorizeCheck(ctx, actor, "read", req.ID); err != nil {
			return err
		}
		row, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := s.requireOrdinaryTicketsTx(ctx, tx, id); err != nil {
			return err
		}
		result, err = s.ticketRepo.HandlingReceiptTx(ctx, tx, actor, key, digest[:])
		if result != nil || err != nil {
			return err
		}
		typ, _, _, err := s.resolveTicketDefinition(ctx, row)
		if err != nil {
			return err
		}
		rule, err := s.handlingAllowed(ctx, action, row, typ, actor)
		if err != nil {
			return err
		}
		if row.Revision != rev {
			return errcode.ErrConcurrentModification
		}
		event := model.TicketEvent{TicketID: id, UserID: actor}
		dispatch := domainevent.DomainEvent{BizType: domainevent.BizTicket, BizID: req.ID, ActorID: actor, Timestamp: time.Now()}
		if action == transferAction {
			if target == *row.AssignedTo {
				return errcode.ErrConflict
			}
			eligible, err := repository.NewTicketPeopleRepo(s.db).EligibleTransferTarget(ctx, target, row.OrgID, *row.AssignedTo)
			if err != nil {
				return err
			}
			if !eligible {
				return errcode.New(errcode.ErrInvalidParams.Code, "转办目标不在本工单可选范围")
			}
			if err := s.ticketRepo.UpdateAssignedToTx(ctx, tx, id, &target); err != nil {
				return err
			}
			event.Action, event.FromValue, event.ToValue = "transferred", strconv.FormatInt(*row.AssignedTo, 10), strconv.FormatInt(target, 10)
			dispatch.Action, dispatch.Payload = domainevent.TicketAssigned, domainevent.Payload{"from_assignee": event.FromValue, "to_assignee": event.ToValue}
			row.AssignedTo = &target
		} else {
			if rule.CommentRequired && strings.TrimSpace(req.Comment) == "" {
				return errcode.New(errcode.ErrInvalidParams.Code, "请填写撤回说明")
			}
			if err := s.ticketRepo.WithdrawUntouchedTx(ctx, tx, id, actor, rev); err != nil {
				return err
			}
			event.Action, event.FromValue, event.ToValue = "withdrawn", StatusOpen, StatusCancelled
			dispatch.Action, dispatch.Payload = domainevent.TicketTransitioned, domainevent.Payload{"from_status": StatusOpen, "to_status": StatusCancelled}
			row.Status = StatusCancelled
		}
		if err := s.ticketRepo.CreateEventTx(ctx, tx, &event); err != nil {
			return err
		}
		if err := s.writeLifecycleCommentTx(ctx, tx, id, actor, req.Comment); err != nil {
			return err
		}
		revision, err := s.ticketRepo.ReadTicketEvidenceRevisionTx(ctx, tx, id)
		if err != nil {
			return err
		}
		result = &model.TicketHandlingResult{ID: id, Revision: revision, Status: row.Status, AssignedTo: row.AssignedTo}
		if err := s.ticketRepo.SaveHandlingReceiptTx(ctx, tx, actor, key, digest[:], result); err != nil {
			return err
		}
		return repository.AfterIAMCommit(ctx, func() error { s.emitTicketEvent(row, dispatch); return nil })
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}

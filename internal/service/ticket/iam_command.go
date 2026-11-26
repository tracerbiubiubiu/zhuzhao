package ticket

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type ticketCommandRolesKey struct{}
type ticketCommandRoles struct {
	actor int64
	roles []string
}

// withHTTPCommand protects the admitted HTTP commands. Existing direct
// service callers (including reviewed automation) retain their L2/L3 contract;
// no absent HTTP identity is promoted to an administrator or an L1 capability.
// fn receives the owning transaction. Callers release all post-commit work
// only after the guard has committed.
func (s *Service) withHTTPCommand(ctx context.Context, action string, actor int64, fn func(context.Context, pgx.Tx) error, ids ...int64) error {
	purpose, http := middleware.IAMRequestFromContext(ctx)
	exclusive := action == "assign" || action == "delete" || action == "cc" || action == "cc/remove"
	run := func(ctx context.Context, tx pgx.Tx) error {
		if riskLifecycleAction(action) {
			// Keep the ordinary condition/validator snapshot. The actual lifecycle
			// writer checks active risk links again once it holds the ticket lock.
			if err := s.admitOrdinaryRiskLifecycleTx(ctx, tx, action, actor, ids...); err != nil {
				return err
			}
		} else if err := s.lockCommandTickets(ctx, tx, ids...); err != nil {
			return err
		}
		return fn(ctx, tx)
	}
	guard := func(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
		if exclusive {
			return repository.WithIAMGuard(ctx, s.db, func(ctx context.Context) error { return fn(ctx, repository.IAMDB(ctx, s.db).(pgx.Tx)) })
		}
		return repository.WithIAMCommandGuard(ctx, s.db, fn)
	}
	if !http {
		// Direct callers keep their existing actor/L2/L3 contract. They join
		// the same ordering barrier without acquiring an HTTP L1 identity.
		return guard(ctx, run)
	}
	path := ""
	selfService := false
	switch action {
	case "create":
		path = ticketListPath
	case "update":
		path = "/api/v1/tickets/update"
	case "transition":
		path = "/api/v1/tickets/transition"
	case "relations":
		path = ticketRelationPath
	case "close", "assign", "delete", "comments", "notes":
		path = "/api/v1/tickets/" + action
	case "cc", "cc/remove", "tags", "tags/remove":
		path = "/api/v1/tickets/" + action
		selfService = true
	case "attachment-presign":
		path = "/api/v1/storage/presign/upload"
		selfService = true
	case "attachment-confirm":
		path = "/api/v1/tickets/attachments/confirm"
		selfService = true
	case "attachment-delete":
		path = "/api/v1/tickets/attachments/delete"
		selfService = true
	}
	if path == "" || purpose.Path != path || purpose.Method != "POST" || purpose.SelfService != selfService || purpose.UserID != actor {
		return errcode.ErrNoPermission
	}
	var routes []ticketPolicyRoute
	if !selfService {
		routes = []ticketPolicyRoute{{path: path, method: "POST"}}
	}
	return guard(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := s.freshCommandCredential(ctx, actor, purpose); err != nil {
			return err
		}
		roles, err := s.freshCommandRoles(ctx, actor, purpose.Model, routes...)
		if err != nil {
			return err
		}
		return run(context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles}), tx)
	})
}

// lockCommandTickets takes row locks after the IAM guard. Preserve the
// caller's business orientation while giving PostgreSQL a stable lock order.
func (s *Service) lockCommandTickets(ctx context.Context, tx pgx.Tx, ids ...int64) error {
	ordered := append([]int64(nil), ids...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	for i, id := range ordered {
		if i > 0 && id == ordered[i-1] {
			continue
		}
		if _, err := s.ticketRepo.GetByIDForUpdateTx(ctx, tx, id); err != nil {
			return err
		}
	}
	return nil
}

type ticketPolicyRoute struct {
	path   string
	method string
}

// freshCommandRoles reads current authorization within the caller's IAM guard.
// Self-service callers pass no route and keep their dynamic resource checks.
func (s *Service) freshCommandRoles(ctx context.Context, actor int64, modelText string, routes ...ticketPolicyRoute) ([]string, error) {
	users := repository.NewUserRepo(s.db)
	account, err := users.FindByID(ctx, actor)
	if errors.Is(err, errcode.ErrUserNotFound) {
		return nil, errcode.ErrNoPermission
	}
	if err != nil {
		return nil, err
	}
	if account.Status != 1 {
		return nil, errcode.ErrNoPermission
	}
	roles, policies, err := users.CurrentIAMAuthorization(ctx, actor)
	if err != nil {
		return nil, fmt.Errorf("fresh ticket authorization: %w", err)
	}
	if len(roles) == 0 {
		return nil, errcode.ErrNoRoles
	}
	if len(routes) == 0 {
		return roles, nil
	}
	grants, err := commandRouteGrants(modelText, roles, policies, routes)
	if err != nil {
		return nil, err
	}
	for _, allowed := range grants {
		if !allowed {
			return nil, errcode.ErrNoPermission
		}
	}
	return roles, nil
}

func commandRouteGrants(modelText string, roles []string, policies [][]string, routes []ticketPolicyRoute) ([]bool, error) {
	m, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return nil, errcode.ErrNoPermission
	}
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		return nil, errcode.ErrNoPermission
	}
	if _, err := enforcer.AddPolicies(policies); err != nil {
		return nil, errcode.ErrNoPermission
	}
	grants := make([]bool, len(routes))
	for i, route := range routes {
		for _, role := range roles {
			granted, err := enforcer.Enforce("role::"+role, route.path, route.method)
			if err != nil {
				return nil, errcode.ErrNoPermission
			}
			if granted {
				grants[i] = true
				break
			}
		}
	}
	return grants, nil
}

// freshCommandCredential runs inside the same IAM ordering and transaction as
// authorization and the business write. Request bodies never supply these facts.
func (s *Service) freshCommandCredential(ctx context.Context, actor int64, purpose middleware.IAMRequestPurpose) error {
	if purpose.SessionEpoch != nil && purpose.PATID == 0 {
		var current int64
		err := repository.IAMDB(ctx, s.db).QueryRow(ctx, `SELECT session_epoch FROM users WHERE id=$1 AND status=1 AND deleted_at IS NULL`, actor).Scan(&current)
		if errors.Is(err, pgx.ErrNoRows) {
			return errcode.ErrNoPermission
		}
		if err != nil {
			return fmt.Errorf("current ticket session: %w", err)
		}
		if current < 1 || current != *purpose.SessionEpoch {
			return errcode.ErrNoPermission
		}
		return nil
	}
	if purpose.PATID > 0 && purpose.SessionEpoch == nil {
		active, err := repository.NewPatRepo(s.db).IsActiveForUser(ctx, purpose.PATID, actor)
		if err != nil {
			return fmt.Errorf("current ticket PAT: %w", err)
		}
		if active {
			return nil
		}
	}
	return errcode.ErrNoPermission
}

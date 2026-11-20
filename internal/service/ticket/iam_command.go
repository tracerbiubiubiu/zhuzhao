package ticket

import (
	"context"
	"errors"
	"fmt"

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

// withHTTPCommand protects only the two admitted HTTP commands. Existing direct
// service callers (including reviewed automation) retain their L2/L3 contract;
// no absent HTTP identity is promoted to an administrator or an L1 capability.
// fn receives the owning transaction. Callers release all post-commit work
// only after the guard has committed.
func (s *Service) withHTTPCommand(ctx context.Context, action string, actor int64, fn func(context.Context, pgx.Tx) error) error {
	purpose, http := middleware.IAMRequestFromContext(ctx)
	if !http {
		return fn(ctx, nil)
	}
	path := ""
	switch action {
	case "update":
		path = "/api/v1/tickets/update"
	case "transition":
		path = "/api/v1/tickets/transition"
	}
	if path == "" || purpose.Path != path || purpose.Method != "POST" || purpose.SelfService || purpose.UserID != actor {
		return errcode.ErrNoPermission
	}
	return repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		users := repository.NewUserRepo(s.db)
		account, err := users.FindByID(ctx, actor)
		if errors.Is(err, errcode.ErrUserNotFound) {
			return errcode.ErrNoPermission
		}
		if err != nil {
			return err
		}
		if account.Status != 1 {
			return errcode.ErrNoPermission
		}
		roles, policies, err := users.CurrentIAMAuthorization(ctx, actor)
		if err != nil {
			return fmt.Errorf("fresh ticket authorization: %w", err)
		}
		if len(roles) == 0 {
			return errcode.ErrNoRoles
		}
		m, err := casbinmodel.NewModelFromString(purpose.Model)
		if err != nil {
			return errcode.ErrNoPermission
		}
		enforcer, err := casbin.NewEnforcer(m)
		if err != nil {
			return errcode.ErrNoPermission
		}
		if _, err := enforcer.AddPolicies(policies); err != nil {
			return errcode.ErrNoPermission
		}
		for _, role := range roles {
			allowed, err := enforcer.Enforce("role::"+role, path, "POST")
			if err != nil {
				return errcode.ErrNoPermission
			}
			if allowed {
				return fn(context.WithValue(ctx, ticketCommandRolesKey{}, ticketCommandRoles{actor: actor, roles: roles}), tx)
			}
		}
		return errcode.ErrNoPermission
	})
}

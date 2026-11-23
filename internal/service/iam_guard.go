package service

import (
	"context"
	"fmt"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type trustedIAMInternalKey struct{}
type trustedIAMActorKey struct{}
type iamCommandKey struct{}

// TrustedIAMInternal explicitly identifies a reviewed non-HTTP caller. It is
// for privileged internal orchestration and direct service test fixtures; no
// production handler or request payload may mint this context.
func TrustedIAMInternal(ctx context.Context) context.Context {
	return context.WithValue(ctx, trustedIAMInternalKey{}, true)
}

// TrustedIAMInternalActor binds a reviewed internal source-maintenance caller.
// No absent identity is promoted to a platform administrator.
func TrustedIAMInternalActor(ctx context.Context, actorID int64) context.Context {
	return context.WithValue(TrustedIAMInternal(ctx), trustedIAMActorKey{}, actorID)
}
func iamCommandActor(ctx context.Context) int64 {
	if trustedIAMInternal(ctx) {
		actorID, _ := ctx.Value(trustedIAMActorKey{}).(int64)
		return actorID
	}
	purpose, _ := middleware.IAMRequestFromContext(ctx)
	return purpose.UserID
}

type iamPurpose struct {
	path string
	self bool
}

// Exact current router mapping; no arbitrary purpose can be supplied by a body.
var iamPurposes = map[string]iamPurpose{
	"user.create": {"/api/v1/users", false}, "user.update": {"/api/v1/users/update", false},
	"user.delete": {"/api/v1/users/delete", false}, "user.status": {"/api/v1/users/status", false},
	"user.roles": {"/api/v1/users/roles", false}, "user.reset": {"/api/v1/users/password/reset", false},
	"user.orgs":   {"/api/v1/users/orgs", false},
	"role.create": {"/api/v1/roles", false}, "role.update": {"/api/v1/roles/update", false},
	"role.delete": {"/api/v1/roles/delete", false}, "role.menus": {"/api/v1/roles/menus", false},
	"org.create": {"/api/v1/orgs", false}, "org.update": {"/api/v1/orgs/update", false}, "org.move": {"/api/v1/orgs/move", false},
	"org.delete": {"/api/v1/orgs/delete", true}, "org.member.add": {"/api/v1/orgs/members", true},
	"org.member.remove": {"/api/v1/orgs/members/delete", true}, "org.member.role": {"/api/v1/orgs/members/role", true},
	"org.member.scope": {"/api/v1/orgs/members/scope", true}, "org.owners": {"/api/v1/orgs/owners", true},
	"org.role.bind": {"/api/v1/orgs/roles/bind", true}, "org.role.unbind": {"/api/v1/orgs/roles/delete", true},
}

func trustedIAMInternal(ctx context.Context) bool {
	trusted, _ := ctx.Value(trustedIAMInternalKey{}).(bool)
	return trusted
}

func iamCommandActive(ctx context.Context, command string) bool {
	value, _ := ctx.Value(iamCommandKey{}).(string)
	return value == command
}

func authorizeIAMCommand(ctx context.Context, command string, actorID int64, users *repository.UserRepo) error {
	if trustedIAMInternal(ctx) {
		return nil
	}
	purpose, ok := middleware.IAMRequestFromContext(ctx)
	expected, known := iamPurposes[command]
	if !ok || !known || purpose.Method != "POST" || purpose.Path != expected.path || purpose.SelfService != expected.self || (actorID != 0 && purpose.UserID != actorID) {
		return errcode.ErrNoPermission
	}
	switch command {
	case "user.status", "user.delete", "user.reset":
		if err := freshC05Credential(ctx, users, purpose); err != nil {
			return err
		}
	default:
		actor, err := users.FindByID(ctx, purpose.UserID)
		if err != nil {
			return err
		}
		if actor.Status != 1 {
			return errcode.ErrNoPermission
		}
	}
	roles, policies, err := currentIAMAuthorization(ctx, users, purpose.UserID)
	if err != nil {
		return err
	}
	return authorizeIAMRoute(roles, policies, expected, purpose.Method, purpose.Model)
}

// freshC05Credential rechecks the credential admitted by middleware using the
// same transaction as the C05 management command or receipt lookup.
func freshC05Credential(ctx context.Context, users *repository.UserRepo, purpose middleware.IAMRequestPurpose) error {
	if purpose.UserID <= 0 {
		return errcode.ErrNoPermission
	}
	account, err := users.IAMAccountSecuritySnapshot(ctx, purpose.UserID)
	if err != nil {
		return err
	}
	if !currentC05AccountCredential(purpose, account) {
		return errcode.ErrNoPermission
	}
	if purpose.PATID == 0 {
		return nil
	}
	active, err := repository.NewPatRepo(users.Pool()).IsActiveForUser(ctx, purpose.PATID, purpose.UserID)
	if err != nil {
		return fmt.Errorf("current C05 PAT: %w", err)
	}
	if !active {
		return errcode.ErrNoPermission
	}
	return nil
}

func currentC05AccountCredential(purpose middleware.IAMRequestPurpose, account repository.AccountSecuritySnapshot) bool {
	if account.Deleted || account.Status != 1 {
		return false
	}
	if purpose.PATID > 0 {
		// PAT lifetime is independent of the account's JWT epoch and Redis projection.
		return purpose.SessionEpoch == nil
	}
	return purpose.PATID == 0 && purpose.SessionEpoch != nil && account.SessionEpoch > 0 && *purpose.SessionEpoch == account.SessionEpoch
}

func currentIAMAuthorization(ctx context.Context, users *repository.UserRepo, actorID int64) ([]string, [][]string, error) {
	roles, policies, err := users.CurrentIAMAuthorization(ctx, actorID)
	if err != nil {
		return nil, nil, fmt.Errorf("fresh IAM authorization: %w", err)
	}
	if len(roles) == 0 {
		return nil, nil, errcode.ErrNoRoles
	}
	return roles, policies, nil
}

func authorizeIAMRoute(roles []string, policies [][]string, expected iamPurpose, method, modelText string) error {
	if expected.self {
		return nil
	}
	m, err := casbinmodel.NewModelFromString(modelText)
	if err != nil {
		return errcode.ErrNoPermission
	}
	enforcer, err := casbin.NewEnforcer(m)
	if err != nil {
		return errcode.ErrNoPermission
	}
	if _, err = enforcer.AddPolicies(policies); err != nil {
		return errcode.ErrNoPermission
	}
	for _, role := range roles {
		allowed, enforceErr := enforcer.Enforce("role::"+role, expected.path, method)
		if enforceErr != nil {
			return errcode.ErrNoPermission
		}
		if allowed {
			return nil
		}
	}
	return errcode.ErrNoPermission
}

func guardedIAMCommand(ctx context.Context, guard func(context.Context, func(context.Context) error) error, command string, actorID int64, users *repository.UserRepo, fn func(context.Context) error) error {
	return guard(ctx, func(ctx context.Context) error {
		if err := authorizeIAMCommand(ctx, command, actorID, users); err != nil {
			return err
		}
		return fn(context.WithValue(ctx, iamCommandKey{}, command))
	})
}
func guardedIAMValue[T any](ctx context.Context, guard func(context.Context, func(context.Context) error) error, command string, actorID int64, users *repository.UserRepo, fn func(context.Context) (T, error)) (value T, err error) {
	err = guardedIAMCommand(ctx, guard, command, actorID, users, func(ctx context.Context) error { var callErr error; value, callErr = fn(ctx); return callErr })
	return value, err
}

// Check only newly materialized membership facts. Global business roles can
// supply an API action but never replace enabled direct IAM assignment tiers.
func (s *OrgService) authorizeNewMemberships(ctx context.Context, actorID, userID int64, orgIDs []int64) error {
	roles, err := s.orgRepo.PotentialMembershipRoles(ctx, userID, orgIDs)
	if err != nil {
		return err
	}
	if len(roles) == 0 {
		return nil
	}
	return s.authorizePotentialRoles(ctx, actorID, roles)
}

func (s *OrgService) authorizePotentialRoles(ctx context.Context, actorID int64, roles []*model.Role) error {
	if len(roles) == 0 {
		return nil
	}
	if actorID <= 0 {
		return errcode.ErrNoPermission
	}
	actorRoles, err := s.userRepo.GetRoles(ctx, actorID)
	if err != nil {
		return err
	}
	for _, role := range roles {
		if !canAssignRole(actorRoles, role) {
			return errcode.ErrCannotAssignHigherRole
		}
	}
	return nil
}
func (s *OrgService) authorizeSourceChange(ctx context.Context, actorID, orgID int64) error {
	roles, err := s.orgRepo.PotentialSubtreeRoles(ctx, orgID)
	if err != nil {
		return err
	}
	return s.authorizePotentialRoles(ctx, actorID, roles)
}

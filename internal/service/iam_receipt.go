package service

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"regexp"
	"slices"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type iamOperationKey struct{}

var iamOperationPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:-]{15,127}$`)

func IAMOperationContext(ctx context.Context, id string) (context.Context, string, error) {
	if id == "" {
		id = uuid.NewString()
	}
	if !iamOperationPattern.MatchString(id) {
		return ctx, "", errcode.ErrInvalidParams
	}
	return context.WithValue(ctx, iamOperationKey{}, id), id, nil
}

func canonicalIAMIDs(input []int64) []int64 {
	ids := append([]int64{}, input...)
	slices.Sort(ids)
	return slices.Compact(ids)
}

func newIAMOperation(ctx context.Context, command string, actor, target int64, ids []int64, original *[]int64) repository.IAMOperation {
	id, _ := ctx.Value(iamOperationKey{}).(string)
	if id == "" {
		id = uuid.NewString()
	}
	input := struct {
		Version       int
		Command       string
		Actor, Target int64
		IDs           []int64
		Original      *[]int64
	}{Version: 1, Command: command, Actor: actor, Target: target, IDs: canonicalIAMIDs(ids)}
	if original != nil {
		v := canonicalIAMIDs(*original)
		input.Original = &v
	}
	raw, _ := json.Marshal(input)
	hash := sha256.Sum256(raw)
	return repository.IAMOperation{ID: id, Command: command, Actor: actor, Target: target, InputHash: hash[:]}
}

func authorizeMenuTarget(role *model.Role, actorRoles []*model.Role) error {
	if role.IsSystem && !isSuperadmin(actorRoles) {
		return errcode.ErrRoleIsSystem
	}
	if !canManageTarget(actorRoles, []*model.Role{role}) {
		return errcode.ErrCannotManageHigher
	}
	return nil
}

func (s *UserService) IAMReceipt(ctx context.Context, actor int64, id string) (*model.IAMReceipt, error) {
	if !iamOperationPattern.MatchString(id) {
		return nil, errcode.ErrInvalidParams
	}
	var result *model.IAMReceipt
	err := repository.WithIAMCommandGuard(ctx, s.db, func(ctx context.Context, tx pgx.Tx) error {
		internal := trustedIAMInternal(ctx)
		purpose, ok := middleware.IAMRequestFromContext(ctx)
		if !internal {
			if !ok || purpose.UserID != actor || !purpose.SelfService || purpose.Method != "GET" || purpose.Path != "/api/v1/operations/receipt" {
				return errcode.ErrNoPermission
			}
			if err := freshC05Credential(ctx, s.userRepo, purpose); err != nil {
				return err
			}
		}
		r, _, err := repository.FindIAMReceipt(ctx, tx, actor, id)
		if err != nil {
			return err
		}
		if r == nil {
			return errcode.ErrNotFound
		}
		command := "user.roles"
		if r.Command == repository.IAMRoleMenus {
			command = "role.menus"
		}
		if !internal {
			roles, policies, err := currentIAMAuthorization(ctx, s.userRepo, actor)
			if err != nil {
				return err
			}
			if err := authorizeIAMRoute(roles, policies, iamPurposes[command], "POST", purpose.Model); err != nil {
				return errcode.ErrNotFound
			}
		}
		if command == "user.roles" {
			if _, err = s.userRepo.FindByID(ctx, r.Target); err == nil {
				err = s.ensureSensitiveCanManage(ctx, actor, r.Target)
			}
		} else {
			var role *model.Role
			role, err = s.roleRepo.FindByID(ctx, r.Target)
			if err == nil {
				var actorRoles []*model.Role
				actorRoles, err = s.userRepo.GetRoles(ctx, actor)
				if err == nil {
					err = authorizeMenuTarget(role, actorRoles)
				}
			}
		}
		if errors.Is(err, errcode.ErrUserNotFound) || errors.Is(err, errcode.ErrRoleNotFound) || errors.Is(err, errcode.ErrRoleIsSystem) || errors.Is(err, errcode.ErrCannotManageHigher) {
			return errcode.ErrNotFound
		}
		if err != nil {
			return err
		}
		result = r
		return nil
	})
	return result, err
}

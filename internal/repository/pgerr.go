package repository

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func mapUniqueViolation(err error) *errcode.Error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		return nil
	}
	switch pgErr.ConstraintName {
	case "idx_users_employee_no":
		return errcode.ErrEmployeeNoAlreadyExists
	case "idx_users_domain_account":
		return errcode.ErrDomainAccountAlreadyExists
	case "idx_org_code":
		return errcode.ErrOrgAlreadyExists
	case "idx_roles_code":
		return errcode.ErrRoleAlreadyExists
	case "idx_menus_code", "menus_code_key": // 000006 迁移前旧约束名兼容
		return errcode.ErrMenuAlreadyExists
	default:
		return errcode.ErrConflict
	}
}

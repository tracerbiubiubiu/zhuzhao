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
	default:
		return errcode.ErrConflict
	}
}

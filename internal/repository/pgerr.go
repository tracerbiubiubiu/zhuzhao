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
	case "idx_user_orgs_single_primary": // B3-3：primary 互斥并发兜底
		return errcode.ErrDuplicatePrimaryOrg
	default:
		return errcode.ErrConflict
	}
}

// MapForeignKeyViolation 外键违规（23503）→ 参数错误。
// B4-3：竞态窗口内（service 预检通过后、INSERT 前）目标角色/组织/用户被物理删除时，
// 关联表写入收到 23503——语义是「关联对象不存在」，400 而非 500。
func MapForeignKeyViolation(err error) *errcode.Error {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
		return nil
	}
	return errcode.ErrInvalidParams
}

package repository

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// B3-3 守护：user_orgs 部分唯一索引约束名 → ErrDuplicatePrimaryOrg（409）
func TestMapUniqueViolation_SinglePrimaryIndex(t *testing.T) {
	err := &pgconn.PgError{Code: "23505", ConstraintName: "idx_user_orgs_single_primary"}
	ec := mapUniqueViolation(err)
	if ec == nil || ec.Code != errcode.ErrDuplicatePrimaryOrg.Code {
		t.Fatalf("want %d, got %v", errcode.ErrDuplicatePrimaryOrg.Code, ec)
	}
}

func TestMapUniqueViolation_OtherConstraint(t *testing.T) {
	// 非唯一冲突错误 → 不映射
	if ec := mapUniqueViolation(errors.New("boom")); ec != nil {
		t.Fatalf("non-pg error should not map, got %v", ec)
	}
	// 未知约束名 → 通用冲突
	err := &pgconn.PgError{Code: "23505", ConstraintName: "unknown_idx"}
	ec := mapUniqueViolation(err)
	if ec == nil || ec.Code != errcode.ErrConflict.Code {
		t.Fatalf("unknown constraint should map to ErrConflict, got %v", ec)
	}
}

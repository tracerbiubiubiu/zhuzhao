package repository

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// AccountSecuritySnapshot 认证中间件消费的最小账号安全事实
// （与 UserSecuritySnapshot 解耦：中间件只看 status/deleted/代际，不触凭据 hash）。
type AccountSecuritySnapshot struct {
	Status          int
	Deleted         bool
	SessionEpoch    int64
	ProjectionEpoch int64
}

// AccountSecurityLookup 供 router 装配进 JWT 中间件（WithAccountSecurity）：
// 每次 Bearer/PAT 认证先取本请求可信 DB 快照——账号 inactive/deleted 为终态拒绝，
// DB 不可读为依赖 503，Redis 缺键不能替代 DB 判断（底座 §4.2 认证读顺序）。
type AccountSecurityLookup struct {
	db interface {
		QueryRow(context.Context, string, ...any) pgx.Row
	}
}

func NewAccountSecurityLookup(db *pgxpool.Pool) *AccountSecurityLookup {
	if db == nil {
		return &AccountSecurityLookup{}
	}
	return &AccountSecurityLookup{db: db}
}

func (l *AccountSecurityLookup) AccountSecuritySnapshot(ctx context.Context, userID int64) (AccountSecuritySnapshot, error) {
	if l == nil || l.db == nil {
		return AccountSecuritySnapshot{}, fmt.Errorf("account security snapshot: database unavailable")
	}
	return scanAccountSecuritySnapshot(l.db.QueryRow(ctx, accountSecuritySnapshotSQL, userID))
}

const accountSecuritySnapshotSQL = `SELECT status, (deleted_at IS NOT NULL), session_epoch, session_projection_epoch
		FROM users WHERE id = $1`

// IAMAccountSecuritySnapshot reads only the account facts needed by the IAM guard.
func (r *UserRepo) IAMAccountSecuritySnapshot(ctx context.Context, userID int64) (AccountSecuritySnapshot, error) {
	return scanAccountSecuritySnapshot(IAMDB(ctx, r.db).QueryRow(ctx, accountSecuritySnapshotSQL, userID))
}

func scanAccountSecuritySnapshot(row pgx.Row) (AccountSecuritySnapshot, error) {
	var s AccountSecuritySnapshot
	err := row.Scan(&s.Status, &s.Deleted, &s.SessionEpoch, &s.ProjectionEpoch)
	if errors.Is(err, pgx.ErrNoRows) {
		return AccountSecuritySnapshot{Deleted: true}, nil
	}
	if err != nil {
		return AccountSecuritySnapshot{}, fmt.Errorf("account security snapshot: %w", err)
	}
	return s, nil
}

package repository

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// PatRepo P4-6 PAT 数据访问。
var (
	ErrPatNotFound = errors.New("repository: pat not found")
)

const patColumns = `id, user_id, name, secret_hash, scope, expires_at, revoked_at, last_used_at, created_at`

type PatRepo struct {
	db *pgxpool.Pool
}

func NewPatRepo(db *pgxpool.Pool) *PatRepo {
	return &PatRepo{db: db}
}

// ListByUser 本人 PAT 列表（含已吊销——审计痕迹；secret_hash 永不出网由 service 剥离）
func (r *PatRepo) ListByUser(ctx context.Context, userID int64) ([]*model.PersonalAccessToken, error) {
	rows, err := r.db.Query(ctx,
		"SELECT "+patColumns+" FROM personal_access_tokens WHERE user_id = $1 ORDER BY id DESC", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.PersonalAccessToken{}
	for rows.Next() {
		var t model.PersonalAccessToken
		if err := rows.Scan(&t.ID, &t.UserID, &t.Name, &t.SecretHash, &t.Scope, &t.ExpiresAt, &t.RevokedAt, &t.LastUsedAt, &t.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, &t)
	}
	return out, rows.Err()
}

// Create 落库（secret_hash 唯一——碰撞即失败，调用方重试）
func (r *PatRepo) Create(ctx context.Context, t *model.PersonalAccessToken) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO personal_access_tokens (user_id, name, secret_hash, scope, expires_at)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id, created_at`,
		t.UserID, t.Name, t.SecretHash, t.Scope, t.ExpiresAt,
	).Scan(&t.ID, &t.CreatedAt)
}

// Revoke 吊销（本人+未吊销；幂等语义=行存在即置 revoked_at）
func (r *PatRepo) Revoke(ctx context.Context, id, userID int64) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE personal_access_tokens SET revoked_at = NOW()
		WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`, id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrPatNotFound
	}
	return nil
}

// FindActiveByHash 认证查验：sha256 命中+未吊销+未过期（二合一 SQL——中间件热路径单查）
// 返回 user_id/user_name 供 context 注入（等效 JWT claims 的 uid/username）。
func (r *PatRepo) FindActiveByHash(ctx context.Context, secretHash string) (userID int64, userName string, patID int64, err error) {
	err = r.db.QueryRow(ctx, `
		SELECT p.user_id, COALESCE(NULLIF(u.real_name, ''), u.username), p.id
		FROM personal_access_tokens p
		JOIN users u ON u.id = p.user_id AND u.deleted_at IS NULL AND u.status = 1
		WHERE p.secret_hash = $1 AND p.revoked_at IS NULL
		  AND (p.expires_at IS NULL OR p.expires_at > NOW())`, secretHash,
	).Scan(&userID, &userName, &patID)
	return
}

// TouchLastUsed 异步更新使用时间（尽力而为——认证路径不因它失败）
func (r *PatRepo) TouchLastUsed(ctx context.Context, patID int64) {
	_, _ = r.db.Exec(ctx,
		"UPDATE personal_access_tokens SET last_used_at = $2 WHERE id = $1", patID, time.Now())
}

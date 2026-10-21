package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// SettingsRepo system_settings KV 读写（000048 B5-6；000050 version 乐观锁）。
type SettingsRepo struct{ db *pgxpool.Pool }

// ErrSettingsVersionConflict 乐观锁冲突（expected_version 与库内不符/竞态重试耗尽）。
var ErrSettingsVersionConflict = errors.New("repository: settings version conflict")

func NewSettingsRepo(db *pgxpool.Pool) *SettingsRepo { return &SettingsRepo{db: db} }

// Get 读单键 KV（不存在零值 nil，不报错——由 catalog/config 兜底）。
func (r *SettingsRepo) Get(ctx context.Context, key string) (*model.SystemSetting, error) {
	s := &model.SystemSetting{}
	err := r.db.QueryRow(ctx,
		`SELECT key, value, updated_by, updated_at, version FROM system_settings WHERE key=$1`, key).
		Scan(&s.Key, &s.Value, &s.UpdatedBy, &s.UpdatedAt, &s.Version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// ListAll 读全部 KV 覆盖行。
func (r *SettingsRepo) ListAll(ctx context.Context) (map[string]*model.SystemSetting, error) {
	rows, err := r.db.Query(ctx, `SELECT key, value, updated_by, updated_at, version FROM system_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*model.SystemSetting{}
	for rows.Next() {
		s := &model.SystemSetting{}
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedBy, &s.UpdatedAt, &s.Version); err != nil {
			return nil, err
		}
		out[s.Key] = s
	}
	return out, rows.Err()
}

// Upsert KV 写入（F-D2 version 乐观锁，2026-10-21——并行审计挂起项收口）：
// expected 非空 = 严格 CAS（与 List 回传版本不符即 ErrSettingsVersionConflict，
// 两个管理员并发改同键时后写者不再静默覆盖）；expected 为空 = 服务端重读重试
// ≤3 次（兼容存量调用，竞态下收敛）。插入路径 ON CONFLICT DO NOTHING + 重读。
func (r *SettingsRepo) Upsert(ctx context.Context, key string, value json.RawMessage, updatedBy int64, expected *int64) error {
	for i := 0; i < 3; i++ {
		cur, err := r.Get(ctx, key)
		if err != nil {
			return err
		}
		if cur == nil {
			tag, err := r.db.Exec(ctx, `
				INSERT INTO system_settings (key, value, updated_by, updated_at, version)
				VALUES ($1, $2, $3, NOW(), 1)
				ON CONFLICT (key) DO NOTHING`, key, value, updatedBy)
			if err != nil {
				return err
			}
			if tag.RowsAffected() == 1 {
				return nil
			}
			continue // 并发已插入——重读走更新路径
		}
		if expected != nil && *expected != cur.Version {
			return ErrSettingsVersionConflict
		}
		tag, err := r.db.Exec(ctx, `
			UPDATE system_settings SET value=$2, updated_by=$3, updated_at=NOW(), version=version+1
			WHERE key=$1 AND version=$4`, key, value, updatedBy, cur.Version)
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 1 {
			return nil
		}
		// 版本被并发改走：expected 提供即冲突（下方 return），否则重读重试
	}
	return ErrSettingsVersionConflict
}

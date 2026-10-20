package repository

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// SettingsRepo system_settings KV 读写（000048 B5-6）。
type SettingsRepo struct{ db *pgxpool.Pool }

func NewSettingsRepo(db *pgxpool.Pool) *SettingsRepo { return &SettingsRepo{db: db} }

// Get 读单键 KV（不存在零值 nil，不报错——由 catalog/config 兜底）。
func (r *SettingsRepo) Get(ctx context.Context, key string) (*model.SystemSetting, error) {
	s := &model.SystemSetting{}
	err := r.db.QueryRow(ctx,
		`SELECT key, value, updated_by, updated_at FROM system_settings WHERE key=$1`, key).
		Scan(&s.Key, &s.Value, &s.UpdatedBy, &s.UpdatedAt)
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
	rows, err := r.db.Query(ctx, `SELECT key, value, updated_by, updated_at FROM system_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]*model.SystemSetting{}
	for rows.Next() {
		s := &model.SystemSetting{}
		if err := rows.Scan(&s.Key, &s.Value, &s.UpdatedBy, &s.UpdatedAt); err != nil {
			return nil, err
		}
		out[s.Key] = s
	}
	return out, rows.Err()
}

// Upsert KV 写入（不存在则插入、存在则覆盖——value 为完整 JSON 值）。
func (r *SettingsRepo) Upsert(ctx context.Context, key string, value json.RawMessage, updatedBy int64) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO system_settings (key, value, updated_by, updated_at)
		VALUES ($1, $2, $3, NOW())
		ON CONFLICT (key) DO UPDATE SET value=$2, updated_by=$3, updated_at=NOW()`,
		key, value, updatedBy)
	return err
}

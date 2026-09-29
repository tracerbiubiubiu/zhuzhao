package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// PanicRepo P4-8 panic 落库+聚合（fingerprint 去重——同因 panic 计数递增不刷屏）。
type PanicRepo struct {
	db *pgxpool.Pool
}

func NewPanicRepo(db *pgxpool.Pool) *PanicRepo {
	return &PanicRepo{db: db}
}

// Upsert 聚合落库（同指纹 count+1+last_at 刷新；ON CONFLICT 幂等并发安全）
func (r *PanicRepo) Upsert(ctx context.Context, fingerprint, message, stack, path string) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO panic_logs (fingerprint, message, stack, path)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (fingerprint) DO UPDATE SET
			count = panic_logs.count + 1, last_at = NOW()`,
		fingerprint, message, stack, path)
	return err
}

// PanicRow 聚合行（fingerprint 不出网——内部聚合键）
type PanicRow struct {
	ID          int64     `json:"id,string"`
	Message     string    `json:"message"`
	Stack       string    `json:"stack"`
	Path        string    `json:"path"`
	Count       int64     `json:"count"`
	FirstAt     time.Time `json:"first_at"`
	LastAt      time.Time `json:"last_at"`
}

// List 最近聚合（last_at DESC 分页）
func (r *PanicRepo) List(ctx context.Context, page, pageSize int) ([]PanicRow, int64, error) {
	var total int64
	if err := r.db.QueryRow(ctx, "SELECT COUNT(*) FROM panic_logs").Scan(&total); err != nil {
		return nil, 0, err
	}
	rows, err := r.db.Query(ctx, `
		SELECT id, message, stack, path, count, first_at, last_at
		FROM panic_logs ORDER BY last_at DESC LIMIT $1 OFFSET $2`, pageSize, (page-1)*pageSize)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	out := []PanicRow{}
	for rows.Next() {
		var p PanicRow
		if err := rows.Scan(&p.ID, &p.Message, &p.Stack, &p.Path, &p.Count, &p.FirstAt, &p.LastAt); err != nil {
			return nil, 0, err
		}
		out = append(out, p)
	}
	return out, total, rows.Err()
}

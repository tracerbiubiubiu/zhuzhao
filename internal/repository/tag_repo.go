// TagRepo 通用标签仓库（S10 B10-2——10 号附录 A.3 ② 表族 CRUD；entity_type 泛型
// 挂载，phase6 风险事件复用同表零改）。
package repository

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type TagRepo struct {
	db *pgxpool.Pool
}

func NewTagRepo(db *pgxpool.Pool) *TagRepo {
	return &TagRepo{db: db}
}

// AddTags 批量挂标签（幂等：UNIQUE(entity_type,entity_id,tag) ON CONFLICT DO NOTHING）。
// 返回**新增**的标签列表；tag 空/超长由 service 预检（repo 只管库）。
func (r *TagRepo) AddTags(ctx context.Context, entityType string, entityID, createdBy int64, tags []string) ([]string, error) {
	rows, err := r.db.Query(ctx, `
		INSERT INTO tags (entity_type, entity_id, tag, created_by)
		SELECT $1, $2, t, $3 FROM unnest($4::text[]) AS t
		ON CONFLICT (entity_type, entity_id, tag) DO NOTHING
		RETURNING tag`, entityType, entityID, createdBy, tags)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var added []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		added = append(added, t)
	}
	// FK 违规在 RETURNING 流上延迟到 rows.Err() 浮出（000062 同款坑）
	if err := rows.Err(); err != nil {
		if ec := MapForeignKeyViolation(err); ec != nil {
			return nil, errcode.New(ec.Code, "标签操作人不存在")
		}
		return nil, err
	}
	return added, nil
}

// RemoveTag 摘标签（幂等）。返回是否实际删除。
func (r *TagRepo) RemoveTag(ctx context.Context, entityType string, entityID int64, tag string) (bool, error) {
	ct, err := r.db.Exec(ctx,
		`DELETE FROM tags WHERE entity_type = $1 AND entity_id = $2 AND tag = $3`,
		entityType, entityID, tag)
	if err != nil {
		return false, err
	}
	return ct.RowsAffected() > 0, nil
}

// ListByEntity 实体标签行（repo 层 join users 富化挂载人——共享标签留痕可溯）。
func (r *TagRepo) ListByEntity(ctx context.Context, entityType string, entityID int64) ([]model.TagRow, error) {
	rows, err := r.db.Query(ctx, `
		SELECT tg.tag, tg.created_by,
		       CASE WHEN u.deleted_at IS NULL THEN COALESCE(NULLIF(u.real_name, ''), u.username) ELSE '' END,
		       CASE WHEN u.deleted_at IS NULL THEN COALESCE(u.employee_no, '') ELSE '' END,
		       tg.created_at
		FROM tags tg
		JOIN users u ON u.id = tg.created_by
		WHERE tg.entity_type = $1 AND tg.entity_id = $2
		ORDER BY tg.id`, entityType, entityID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (model.TagRow, error) {
		var t model.TagRow
		err := row.Scan(&t.Tag, &t.CreatedBy, &t.CreatedByName, &t.CreatedByNo, &t.CreatedAt)
		return t, err
	})
}

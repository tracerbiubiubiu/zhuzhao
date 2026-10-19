package repository

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// StorageRepo file_objects 元数据访问（phase5 S5 B5-1；对象本体在 RustFS）。
// 错误形态对齐仓内范式：sentinel 由 service 映射 errcode。
var (
	ErrStorageObjectNotFound = errors.New("repository: file object not found")
	ErrStorageAlreadyBound   = errors.New("repository: file object already confirmed/bound")
)

const fileObjectColumns = `id, bucket, object_key, purpose, status, file_name, mime_type, size_bytes, etag, created_by, created_at, confirmed_at`

type StorageRepo struct {
	db *pgxpool.Pool
}

func NewStorageRepo(db *pgxpool.Pool) *StorageRepo {
	return &StorageRepo{db: db}
}

// InsertPending presign 即写 pending（05 号 A.3 修正②——孤儿对象 GC 锚点）。
func (r *StorageRepo) InsertPending(ctx context.Context, f *model.FileObject) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, created_at`,
		f.Bucket, f.ObjectKey, f.Purpose, f.Status, f.FileName, f.MIMEType, f.CreatedBy,
	).Scan(&f.ID, &f.CreatedAt)
}

// GetByObjectKey 按 (bucket, object_key) 唯一定位。
func (r *StorageRepo) GetByObjectKey(ctx context.Context, bucket, objectKey string) (*model.FileObject, error) {
	f := &model.FileObject{}
	err := r.db.QueryRow(ctx,
		"SELECT "+fileObjectColumns+" FROM file_objects WHERE bucket = $1 AND object_key = $2",
		bucket, objectKey,
	).Scan(&f.ID, &f.Bucket, &f.ObjectKey, &f.Purpose, &f.Status, &f.FileName, &f.MIMEType,
		&f.SizeBytes, &f.ETag, &f.CreatedBy, &f.CreatedAt, &f.ConfirmedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrStorageObjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return f, nil
}

// Confirm 条件更新 pending→confirmed 并回填 HEAD 实测（size/etag——拍板 1b 列快照）。
// 0 行受影响 = 已非 pending（重复 confirm / 已绑定）→ ErrStorageAlreadyBound 哨兵。
func (r *StorageRepo) Confirm(ctx context.Context, id int64, sizeBytes int64, etag string) error {
	tag, err := r.db.Exec(ctx, `
		UPDATE file_objects
		SET status = 'confirmed', size_bytes = $2, etag = $3, confirmed_at = NOW()
		WHERE id = $1 AND status = 'pending'`,
		id, sizeBytes, etag)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrStorageAlreadyBound
	}
	return nil
}

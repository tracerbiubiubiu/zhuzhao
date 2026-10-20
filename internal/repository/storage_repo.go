package repository

import (
	"context"
	"errors"

	"time"

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

// ListExpiredPending 孤儿 GC 候选（05 号 A.3 修正②——部分索引 idx_file_objects_gc 命中；
// limit 单批有界，ReentrantJob 收敛语义：一轮清不完下轮续）。
func (r *StorageRepo) ListExpiredPending(ctx context.Context, bucket string, before time.Time, limit int) ([]*model.FileObject, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+fileObjectColumns+` FROM file_objects
		WHERE bucket = $1 AND status = 'pending' AND created_at < $2
		ORDER BY id LIMIT $3`, bucket, before, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.FileObject{}
	for rows.Next() {
		f := &model.FileObject{}
		if err := rows.Scan(&f.ID, &f.Bucket, &f.ObjectKey, &f.Purpose, &f.Status, &f.FileName,
			&f.MIMEType, &f.SizeBytes, &f.ETag, &f.CreatedBy, &f.CreatedAt, &f.ConfirmedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// MarkDeleted GC 落标（条件更新 pending→deleted；0 行=已处理，幂等）。
func (r *StorageRepo) MarkDeleted(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx,
		`UPDATE file_objects SET status = 'deleted' WHERE id = $1 AND status = 'pending'`, id)
	return err
}

// GetUserAvatar 读当前头像对象键（avatar 回写链——users 表跨表读，
// repository 平铺包域内直查；换后端形态时随 avatar 链整体迁移）。
func (r *StorageRepo) GetUserAvatar(ctx context.Context, userID int64) (string, error) {
	var avatar string
	err := r.db.QueryRow(ctx, `SELECT coalesce(avatar, '') FROM users WHERE id = $1`, userID).Scan(&avatar)
	if err != nil {
		return "", err
	}
	return avatar, nil
}

// UpdateUserAvatar 回写头像对象键。
func (r *StorageRepo) UpdateUserAvatar(ctx context.Context, userID int64, objectKey string) error {
	_, err := r.db.Exec(ctx, `UPDATE users SET avatar = $2 WHERE id = $1`, userID, objectKey)
	return err
}

// MarkAvatarDropped 旧头像弃旧（A.7：换头像即弃旧 GC 收——条件限定
// purpose=avatar，防止误标他域对象）。
func (r *StorageRepo) MarkAvatarDropped(ctx context.Context, bucket, objectKey string) error {
	_, err := r.db.Exec(ctx,
		`UPDATE file_objects SET status = 'deleted' WHERE bucket = $1 AND object_key = $2 AND purpose = 'avatar'`,
		bucket, objectKey)
	return err
}

// ConfirmAndBind ticket 态确认并落绑（repo 内单事务：条件翻 confirmed +
// INSERT ticket_attachments + purpose 归一；UNIQUE 冲突/非 pending →
// ErrStorageAlreadyBound）。B5-3 批2。
func (r *StorageRepo) ConfirmAndBind(ctx context.Context, fileID, ticketID, uploadedBy int64, sizeBytes int64, etag string) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE file_objects SET status='confirmed', size_bytes=$2, etag=$3, confirmed_at=NOW()
		WHERE id=$1 AND status='pending'`, fileID, sizeBytes, etag)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrStorageAlreadyBound
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by)
		VALUES ($1, $2, $3)`, ticketID, fileID, uploadedBy); err != nil {
		if isPgUnique(err) {
			return ErrStorageAlreadyBound
		}
		return err
	}
	return tx.Commit(ctx)
}

// BindCreateAttachments 建单前绑定（A.5 建单前链）：悬挂 confirmed 对象批量
// 校验+落绑+purpose 翻正式态。校验失败返回 ErrStorageObjectNotFound（对象
// 不存在/非本人/非 confirmed/悬挂态不符/已绑定——建单方参数错，service 统一
// 映射 400）。B5-3 批2；exec 由调用方事务传入（CreateTicket 同事务）。
func (r *StorageRepo) BindCreateAttachments(ctx context.Context, exec rowExec, userID, ticketID int64, keys []string) error {
	for _, key := range keys {
		var id int64
		var purpose string
		var createdBy int64
		var status string
		err := exec.QueryRow(ctx, `
			SELECT id, purpose, created_by, status FROM file_objects
			WHERE object_key=$1`, key).
			Scan(&id, &purpose, &createdBy, &status)
		if err != nil {
			return ErrStorageObjectNotFound
		}
		if createdBy != userID || status != "confirmed" || purpose == "" {
			return ErrStorageObjectNotFound
		}
		formal := ""
		switch purpose {
		case "attachment_create":
			formal = "ticket_attachment"
		case "richtext_create":
			formal = "ticket_richtext"
		default:
			return ErrStorageObjectNotFound // 非悬挂态对象不可建单落绑
		}
		if _, err := exec.Exec(ctx, `
			INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by)
			VALUES ($1, $2, $3)`, ticketID, id, userID); err != nil {
			return err
		}
		if _, err := exec.Exec(ctx, `
			UPDATE file_objects SET purpose=$3 WHERE id=$1 AND purpose=$2`, id, purpose, formal); err != nil {
			return err
		}
	}
	return nil
}

// ListAttachmentsByTicket 附件列表视图（JOIN file_objects——B5-3 批3）。
func (r *StorageRepo) ListAttachmentsByTicket(ctx context.Context, ticketID int64) ([]*model.TicketAttachmentView, error) {
	rows, err := r.db.Query(ctx, `
		SELECT ta.id, ta.ticket_id, fo.object_key, fo.file_name, fo.mime_type, fo.size_bytes, ta.uploaded_by, ta.created_at
		FROM ticket_attachments ta
		JOIN file_objects fo ON fo.id = ta.file_object_id
		WHERE ta.ticket_id = $1 ORDER BY ta.id`, ticketID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.TicketAttachmentView{}
	for rows.Next() {
		v := &model.TicketAttachmentView{}
		if err := rows.Scan(&v.ID, &v.TicketID, &v.ObjectKey, &v.FileName, &v.MIMEType, &v.SizeBytes, &v.UploadedBy, &v.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// GetBoundTicketID 对象绑定的工单（无绑定 → ErrStorageObjectNotFound；
// 下载面 L2 判定锚点——B5-3 批3）。
func (r *StorageRepo) GetBoundTicketID(ctx context.Context, objectKey string) (int64, error) {
	var ticketID int64
	err := r.db.QueryRow(ctx, `
		SELECT ta.ticket_id FROM ticket_attachments ta
		JOIN file_objects fo ON fo.id = ta.file_object_id
		WHERE fo.object_key = $1 LIMIT 1`, objectKey).Scan(&ticketID)
	if err != nil {
		return 0, ErrStorageObjectNotFound
	}
	return ticketID, nil
}

// UnlinkAttachment 删关联行（0 行 → ErrStorageObjectNotFound）。
func (r *StorageRepo) UnlinkAttachment(ctx context.Context, ticketID int64, objectKey string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM ticket_attachments
		WHERE ticket_id = $1 AND file_object_id = (SELECT id FROM file_objects WHERE object_key = $2)`,
		ticketID, objectKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrStorageObjectNotFound
	}
	return nil
}

// MarkObjectDeleted 对象弃旧标 deleted（通用——avatar/附件删除共用）。
func (r *StorageRepo) MarkObjectDeleted(ctx context.Context, objectKey string) error {
	_, err := r.db.Exec(ctx, `UPDATE file_objects SET status = 'deleted' WHERE object_key = $1`, objectKey)
	return err
}

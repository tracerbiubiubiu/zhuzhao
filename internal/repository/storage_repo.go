package repository

import (
	"context"
	"errors"
	"strconv"

	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// StorageRepo file_objects 元数据访问（phase5 S5 B5-1；对象本体在 RustFS）。
// 错误形态对齐仓内范式：sentinel 由 service 映射 errcode。
var (
	ErrStorageObjectNotFound  = errors.New("repository: file object not found")
	ErrStorageAlreadyBound    = errors.New("repository: file object already confirmed/bound")
	ErrStorageAttachmentLimit = errors.New("repository: ticket attachment limit exceeded")
)

const fileObjectColumns = `id, bucket, object_key, purpose, status, file_name, mime_type, size_bytes, etag, created_by, created_at, confirmed_at, owner_kind, owner_id`

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
		&f.SizeBytes, &f.ETag, &f.CreatedBy, &f.CreatedAt, &f.ConfirmedAt, &f.OwnerKind, &f.OwnerID)
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
			&f.MIMEType, &f.SizeBytes, &f.ETag, &f.CreatedBy, &f.CreatedAt, &f.ConfirmedAt, &f.OwnerKind, &f.OwnerID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteRowIfPending GC pending 路径物理清行（条件删除 pending 态；0 行=已被
// 并发 confirm 翻态不动——A.9「对象与行均消失」终态。2026-10-20 删除口径对齐批：
// 原 MarkDeleted 留行为中间态，pending 孤儿路径改直达终态）。
func (r *StorageRepo) DeleteRowIfPending(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM file_objects WHERE id = $1 AND status = 'pending'`, id)
	return err
}

// ListDeleted 二扫候选：删附件/弃旧流程 best-effort 物理删失败后的 deleted 行
// （对象残留）——重删成功后行物理清除（S5 评审 P2-1：修复前 deleted 态无任何
// 扫描覆盖 = 对象永久无主泄漏）。重试语义无时间窗，ORDER BY id 单批有界收敛。
func (r *StorageRepo) ListDeleted(ctx context.Context, bucket string, limit int) ([]*model.FileObject, error) {
	rows, err := r.db.Query(ctx, `
		SELECT `+fileObjectColumns+` FROM file_objects
		WHERE bucket = $1 AND status = 'deleted'
		ORDER BY id LIMIT $2`, bucket, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*model.FileObject{}
	for rows.Next() {
		f := &model.FileObject{}
		if err := rows.Scan(&f.ID, &f.Bucket, &f.ObjectKey, &f.Purpose, &f.Status, &f.FileName,
			&f.MIMEType, &f.SizeBytes, &f.ETag, &f.CreatedBy, &f.CreatedAt, &f.ConfirmedAt, &f.OwnerKind, &f.OwnerID); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// DeleteRowById 二扫路径物理清行（deleted 为终态无并发翻态，无条件删；
// FK 撞残留关联行时报错由调用方 Warn 留待下轮自愈）。
func (r *StorageRepo) DeleteRowById(ctx context.Context, id int64) error {
	_, err := r.db.Exec(ctx, `DELETE FROM file_objects WHERE id = $1`, id)
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

// SetObjectOwner 置对象通用归属（000047 随行件）。avatar bind 填 user；ticket 态由
// ConfirmAndBind / BindCreateAttachments 在翻态语句内同步落 ticket，不经本方法。
// 0 行 → ErrStorageObjectNotFound（(bucket, object_key) 唯一键对齐）。
func (r *StorageRepo) SetObjectOwner(ctx context.Context, bucket, objectKey, ownerKind, ownerID string) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE file_objects SET owner_kind = $3, owner_id = $4 WHERE bucket = $1 AND object_key = $2`,
		bucket, objectKey, ownerKind, ownerID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrStorageObjectNotFound
	}
	return nil
}

// ConfirmAndBind ticket 态确认并落绑（repo 内单事务：同单行锁串行化并发绑定 +
// 附件数计数校验（05 号 §3 个数上限执行点）+ 条件翻 confirmed + INSERT
// ticket_attachments + purpose 归一；UNIQUE 冲突/非 pending →
// ErrStorageAlreadyBound；超上限 → ErrStorageAttachmentLimit）。B5-3 批2；
// 并发加固 + 计数校验 2026-10-20（S5 评审 P1）。
func (r *StorageRepo) ConfirmAndBind(ctx context.Context, fileID, ticketID, uploadedBy int64, sizeBytes int64, etag string, maxCount int) error {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	// 同单行锁：串行化并发 confirm 绑定同一工单（否则计数校验有 TOCTOU 窗口——
	// 两个并发各见 N-1 均过校验则 N+1 超限）。锁序：tickets → file_objects 恒定。
	var lockID int64
	if err := tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticketID).Scan(&lockID); err != nil {
		return err
	}
	var n int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, ticketID).Scan(&n); err != nil {
		return err
	}
	if maxCount > 0 && n+1 > maxCount { // 与 BindCreateAttachments 的 n+len 同形（审计 M-10）
		return ErrStorageAttachmentLimit
	}

	tag, err := tx.Exec(ctx, `
		UPDATE file_objects SET status='confirmed', size_bytes=$2, etag=$3, confirmed_at=NOW(),
		       owner_kind=$4, owner_id=$5
		WHERE id=$1 AND status='pending'`, fileID, sizeBytes, etag, model.FileOwnerTicket, strconv.FormatInt(ticketID, 10))
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
// 计数校验+加锁校验+落绑+purpose 翻正式态。校验失败返回 ErrStorageObjectNotFound
// （对象不存在/非本人/非 confirmed/悬挂态不符/已被并发他单绑定——建单方参数错，
// service 统一映射 400；仅 pgx.ErrNoRows 转哨兵，DB 故障原样上抛走 500 面
// ——S5 评审 P2-2）。B5-3 批2；exec 由调用方事务传入（CreateTicket 同事务）；
// 并发加固 + 计数校验 + bucket 条件 2026-10-20（S5 评审 P1/D1-3）。
func (r *StorageRepo) BindCreateAttachments(ctx context.Context, exec rowExec, bucket string, userID, ticketID int64, keys []string, maxCount int) error {
	// 附件数计数（建单路径执行点）：本次 keys 与已有合计不超上限（重复键由 UNIQUE 兜底——审计 M-7 措辞对齐）
	var n int
	if err := exec.QueryRow(ctx,
		`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, ticketID).Scan(&n); err != nil {
		return err
	}
	if maxCount > 0 && n+len(keys) > maxCount {
		return ErrStorageAttachmentLimit
	}
	for _, key := range keys {
		var id int64
		var purpose string
		var createdBy int64
		var status string
		// FOR UPDATE：对象行锁串行化并发建单——后到事务阻塞至此单提交后重读，
		// purpose 已翻正式态 → switch default 拒绝（修复前双双落绑）
		err := exec.QueryRow(ctx, `
			SELECT id, purpose, created_by, status FROM file_objects
			WHERE bucket=$1 AND object_key=$2 FOR UPDATE`, bucket, key).
			Scan(&id, &purpose, &createdBy, &status)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return ErrStorageObjectNotFound
			}
			return err
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
			if isPgUnique(err) {
				return ErrStorageAlreadyBound
			}
			return err
		}
		tag, err := exec.Exec(ctx, `
			UPDATE file_objects SET purpose=$3, owner_kind=$4, owner_id=$5 WHERE id=$1 AND purpose=$2`,
			id, purpose, formal, model.FileOwnerTicket, strconv.FormatInt(ticketID, 10))
		if err != nil {
			return err
		}
		if tag.RowsAffected() == 0 {
			// 并发兜底（锁异常路径）：对象已被翻正式态，拒绝本单落绑
			return ErrStorageObjectNotFound
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
// 下载面 L2 判定锚点——B5-3 批3。bucket 条件 + 非 ErrNoRows 错误上抛
// ——S5 评审 P2-2/D1-3 健壮性批 2026-10-20：原裸查跨桶命中、DB 故障吞成 404）。
func (r *StorageRepo) GetBoundTicketID(ctx context.Context, bucket, objectKey string) (int64, error) {
	var ticketID int64
	err := r.db.QueryRow(ctx, `
		SELECT ta.ticket_id FROM ticket_attachments ta
		JOIN file_objects fo ON fo.id = ta.file_object_id
		WHERE fo.bucket = $1 AND fo.object_key = $2 LIMIT 1`, bucket, objectKey).Scan(&ticketID)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrStorageObjectNotFound
	}
	if err != nil {
		return 0, err
	}
	return ticketID, nil
}

// UnlinkAttachment 删关联行（0 行 → ErrStorageObjectNotFound；bucket 条件
// 与 (bucket,object_key) 唯一键对齐——S5 评审 D1-3 健壮性批）。
func (r *StorageRepo) UnlinkAttachment(ctx context.Context, bucket string, ticketID int64, objectKey string) error {
	tag, err := r.db.Exec(ctx, `
		DELETE FROM ticket_attachments
		WHERE ticket_id = $1 AND file_object_id = (SELECT id FROM file_objects WHERE bucket = $2 AND object_key = $3)`,
		ticketID, bucket, objectKey)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrStorageObjectNotFound
	}
	return nil
}

// MarkObjectDeleted 对象弃旧标 deleted（附件删除路径专用——审计 M-5：补 bucket
// 与 purpose 双条件，防多桶/key 碰撞误标他域对象；avatar 弃旧走 MarkAvatarDropped）。
func (r *StorageRepo) MarkObjectDeleted(ctx context.Context, bucket, objectKey string) error {
	_, err := r.db.Exec(ctx, `
		UPDATE file_objects SET status = 'deleted'
		WHERE bucket = $1 AND object_key = $2
		  AND purpose IN ('ticket_attachment', 'ticket_richtext')`, bucket, objectKey)
	return err
}

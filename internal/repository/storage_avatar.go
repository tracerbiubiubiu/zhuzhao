package repository

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// BindAvatar 原子切换用户头像并退役旧键，返回可物理清理的旧键（空串=无弃旧）。
// 锁序 users → 新 file_objects → 旧 file_objects：同一用户的绑定串行化，
// 对象状态在持锁后重验；GC 仅锁对象，不反向取用户锁。归属与引用同事务提交。
func (r *StorageRepo) BindAvatar(ctx context.Context, bucket, objectKey string, userID int64) (string, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(ctx)

	var old string
	if err := tx.QueryRow(ctx, `SELECT coalesce(avatar, '') FROM users WHERE id = $1 FOR UPDATE`, userID).Scan(&old); err != nil {
		return "", err
	}
	if err := bindAvatarObject(ctx, tx, bucket, objectKey, userID); err != nil {
		return "", err
	}
	// Avatar binding and metadata edits share the users.version generation.
	// A profile read before this switch must fail CAS instead of restoring the
	// retired old key. Rebinding the current key is a no-op for that generation.
	if old != objectKey {
		if _, err := tx.Exec(ctx, `UPDATE users SET avatar = $2, version = version + 1, updated_at = NOW() WHERE id = $1`, userID, objectKey); err != nil {
			return "", err
		}
	}
	var retired string
	if old != "" && old != objectKey {
		retired, err = retireAvatarObject(ctx, tx, bucket, old, userID)
		if err != nil {
			return "", err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return "", err
	}
	return retired, nil
}

// Caller holds the target user row. Empty legacy owner metadata is accepted
// only alongside the original uploader; contradictory ownership never transfers.
func bindAvatarObject(ctx context.Context, tx pgx.Tx, bucket, key string, userID int64) error {
	var id int64
	want := strconv.FormatInt(userID, 10)
	// Exclude attachment/pending/foreign rows BEFORE attempting their lock:
	// confirmation holds object→users FK; avatar binding holds users→object.
	err := tx.QueryRow(ctx, `SELECT id FROM file_objects
 WHERE bucket=$1 AND object_key=$2 AND created_by=$3
 AND status='confirmed' AND purpose='avatar'
 AND ((coalesce(owner_kind,'')='' AND coalesce(owner_id,'')='') OR
      (owner_kind='user' AND owner_id=$4)) FOR UPDATE`, bucket, key, userID, want).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrStorageObjectNotFound
	}
	if err != nil {
		return err
	}

	_, err = tx.Exec(ctx, `UPDATE file_objects SET owner_kind=$2,owner_id=$3 WHERE id=$1`, id, model.FileOwnerUser, want)
	return err
}

func retireAvatarObject(ctx context.Context, tx pgx.Tx, bucket, old string, userID int64) (string, error) {
	var internal bool
	old, internal = NormalizeAvatar(old)
	if !internal {
		return "", nil
	}
	var id int64
	// Prove target ownership before taking an old-object lock; never retire an
	// unrelated historical key merely because the target's profile contains it.
	err := tx.QueryRow(ctx, `SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2
 AND created_by=$3 AND purpose='avatar' AND status='confirmed'
 AND ((coalesce(owner_kind,'')='' AND coalesce(owner_id,'')='') OR
      (owner_kind='user' AND owner_id=$3::bigint::text)) FOR UPDATE`, bucket, old, userID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	// Fresh statement after the lock sees proxy/raw references committed while waiting.
	keys, err := avatarReferenceKeys(ctx, tx)
	if err != nil {
		return "", err
	}
	tag, err := tx.Exec(ctx, `UPDATE file_objects SET status = 'deleted' WHERE id=$1
 AND NOT (object_key = ANY($2::text[]))`, id, keys)
	if err != nil {
		return "", err
	}
	if tag.RowsAffected() > 0 {
		return old, nil
	}
	return "", nil
}

// MarkAvatarOrphanDeleted 与绑定器共用对象行锁。先锁、再以新语句检查 users 引用：
// 单条 UPDATE 的 NOT EXISTS 快照可能早于锁等待，无法看到绑定事务刚提交的新引用。
// GC 赢锁时把状态退役，绑定器随后重验拒绝；绑定器赢锁时 GC 新快照让位。
func (r *StorageRepo) MarkAvatarOrphanDeleted(ctx context.Context, id int64, objectKey string) (bool, error) {
	tx, err := r.db.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	var lockID int64
	err = tx.QueryRow(ctx, `SELECT id FROM file_objects WHERE id = $1 AND object_key = $2 FOR UPDATE`, id, objectKey).Scan(&lockID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	keys, err := avatarReferenceKeys(ctx, tx)
	if err != nil {
		return false, err
	}
	tag, err := tx.Exec(ctx, `UPDATE file_objects SET status = 'deleted'
        WHERE id = $1 AND status = 'confirmed' AND purpose = 'avatar'
          AND NOT (object_key = ANY($2::text[]))`, id, keys)
	if err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

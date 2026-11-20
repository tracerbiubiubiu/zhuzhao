package repository

import (
	"context"
	"errors"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// NormalizeAvatar recognizes the existing reserved proxy route plus avatar
// namespace. There is no configured application origin: absolute HTTP(S) URLs
// with this exact dedicated path use the same policy, regardless of host.
// Other external HTTP(S) URLs remain external. Parse the path once, never infer
// ownership from a URL's query, fragment or a key-looking suffix.
func NormalizeAvatar(value string) (string, bool) {
	if value == "" {
		return value, false
	}
	u, err := url.Parse(value)
	if err == nil && strings.HasPrefix(u.Path, "/api/v1/files/avatars/") {
		key := strings.TrimPrefix(u.Path, "/api/v1/files/")
		if validAvatarKey(key) && u.Opaque == "" && (u.Scheme == "" || u.Scheme == "http" || u.Scheme == "https") {
			return key, true
		}
		return value, true // invalid reserved path cannot fall through as external
	}
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return value, false
	}
	return value, true
}

func validAvatarKey(key string) bool {
	if !strings.HasPrefix(key, "avatars/") || strings.ContainsAny(key, "\\\x00") {
		return false
	}
	for _, part := range strings.Split(key, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func InternalAvatar(value string) bool { _, internal := NormalizeAvatar(value); return internal }

// avatarReferenceKeys preserves historical raw and proxy spellings, including
// soft-deleted users. Shared by listing and fresh post-object-lock checks.
func avatarReferenceKeys(ctx context.Context, db IAMDBTX) ([]string, error) {
	rows, err := db.Query(ctx, `SELECT avatar FROM users WHERE avatar IS NOT NULL AND avatar <> ''`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		if key, internal := NormalizeAvatar(value); internal {
			keys = append(keys, key)
		}
	}
	return keys, rows.Err()
}

// UpdateWithAvatar joins explicit internal-key binding and the profile CAS in
// one transaction (savepoint under the admin IAM guard), scoped to runtime bucket.
func (r *UserRepo) UpdateWithAvatar(ctx context.Context, user *model.User, profile bool, bucket string) error {
	if !profile && !IAMWriteActive(ctx) {
		return r.WithIAMGuard(ctx, func(ctx context.Context) error { return r.UpdateWithAvatar(ctx, user, profile, bucket) })
	}
	tx, err := IAMDB(ctx, r.db).Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var old string
	var version int
	err = tx.QueryRow(ctx, `SELECT coalesce(avatar,''),version FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, user.ID).Scan(&old, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return errcode.ErrConcurrentModification
	}
	if err != nil {
		return err
	}
	if version != user.Version {
		return errcode.ErrConcurrentModification
	}
	if err = bindAvatarObject(ctx, tx, bucket, user.Avatar, user.ID); err != nil {
		if errors.Is(err, ErrStorageObjectNotFound) {
			return errcode.ErrFileNotFound
		}
		return err
	}
	// The new reference must be visible before retirement checks all spellings.
	if profile {
		err = updateUserProfile(ctx, tx, user)
	} else {
		err = updateUser(ctx, tx, user)
	}
	if err != nil {
		return err
	}
	if old != "" && old != user.Avatar {
		if _, err = retireAvatarObject(ctx, tx, bucket, old, user.ID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

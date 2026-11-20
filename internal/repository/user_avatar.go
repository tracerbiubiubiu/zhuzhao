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
// Other external HTTP(S) URLs remain external. Fold browser URL paths before
// decoding once, never infer ownership from a query, fragment or key-like suffix.
// Raw object keys are not URL paths and must never be cleaned or decoded.
func NormalizeAvatar(value string) (string, bool) {
	if value == "" {
		return value, false
	}
	// Browsers ignore surrounding C0/space and embedded ASCII tabs/newlines.
	// Apply that behavior only when the result denotes a URL, never to raw keys.
	input := strings.TrimFunc(value, func(r rune) bool { return r <= ' ' })
	input = strings.NewReplacer("\t", "", "\r", "", "\n", "").Replace(input)
	lower := strings.ToLower(input)
	httpURL := strings.HasPrefix(lower, "http:") || strings.HasPrefix(lower, "https:")
	if strings.HasPrefix(input, "/") || strings.HasPrefix(input, "\\") || httpURL {
		// A literal backslash is a separator for HTTP(S), unlike an escaped
		// %5C. Do this before Parse so authority/path boundaries also agree.
		end := strings.IndexAny(input, "?#")
		if end < 0 {
			end = len(input)
		}
		address := strings.ReplaceAll(input[:end], "\\", "/")
		// Special HTTP(S) URLs ignore excess authority-leading slashes.
		// This does not merge any slashes within the actual path.
		if httpURL {
			if i := strings.Index(address, "://"); i >= 0 {
				address = address[:i+3] + strings.TrimLeft(address[i+3:], "/")
			}
		} else if strings.HasPrefix(address, "//") {
			address = "//" + strings.TrimLeft(address, "/")
		}
		input = address + input[end:]
	} else {
		return value, true
	}
	u, err := url.Parse(input)
	if err == nil && u.Opaque == "" && (u.Scheme == "" || u.Scheme == "http" || u.Scheme == "https") {
		proxyPath, pathErr := url.PathUnescape(avatarBrowserPath(u.EscapedPath()))
		if pathErr == nil && strings.HasPrefix(proxyPath, "/api/v1/files/avatars/") {
			key := strings.TrimPrefix(proxyPath, "/api/v1/files/")
			if validAvatarKey(key) {
				return key, true
			}
			return value, true // invalid reserved path cannot fall through as external
		}
	}
	if err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != "" {
		return value, false
	}
	return value, true
}

// Fold dot segments while they are still URL segments. Cleaning after decoding
// would incorrectly fold escaped slashes/backslashes too. Keep empty segments:
// nginx proxy_pass has no URI and forwards the original URI to Gin, so its
// normalized location matching does not make repeated slashes equivalent keys.
func avatarBrowserPath(escaped string) string {
	parts := strings.Split(escaped, "/")
	out := make([]string, 0, len(parts))
	for i, part := range parts {
		dot := strings.ReplaceAll(strings.ToLower(part), "%2e", ".")
		switch dot {
		case ".", "..":
			if dot == ".." && len(out) > 1 {
				out = out[:len(out)-1]
			}
			if i == len(parts)-1 {
				out = append(out, "")
			}
		default:
			out = append(out, part)
		}
	}
	return strings.Join(out, "/")
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
		key, internal := NormalizeAvatar(value)
		if !internal {
			continue
		}
		keys = append(keys, key)
		// The UI puts its proxy prefix in front of a historical bare value.
		// Preserve the browser's resolved target as well as the literal value.
		if key == value && !strings.HasPrefix(value, "/") &&
			(!validAvatarKey(value) || strings.ContainsAny(value, "%?#\t\r\n")) {
			rendered, _ := NormalizeAvatar("/api/v1/files/" + value)
			if rendered != key && validAvatarKey(rendered) {
				keys = append(keys, rendered)
			}
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

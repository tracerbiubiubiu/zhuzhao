//go:build integration

package service_test

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// The storage port records successful physical retirement; all database and
// profile/avatar service calls below are real, including their transactions.
type avatarProfileClient struct {
	mu   sync.Mutex
	keys map[string]bool
}

func (*avatarProfileClient) PresignPut(context.Context, string, string, time.Duration) (*pkgstorage.PresignedPut, error) {
	return nil, fmt.Errorf("unused presign")
}
func (*avatarProfileClient) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", fmt.Errorf("unused presign")
}
func (c *avatarProfileClient) StatObject(_ context.Context, key string) (*pkgstorage.ObjectInfo, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.keys[key] {
		return nil, pkgstorage.ErrObjectNotFound
	}
	return &pkgstorage.ObjectInfo{Key: key}, nil
}
func (c *avatarProfileClient) GetObject(ctx context.Context, key, _ string) (io.ReadCloser, *pkgstorage.ObjectInfo, error) {
	info, err := c.StatObject(ctx, key)
	if err != nil {
		return nil, nil, err
	}
	return io.NopCloser(strings.NewReader("avatar")), info, nil
}
func (c *avatarProfileClient) RemoveObject(_ context.Context, key string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.keys, key)
	return nil
}

func TestUserAvatarProfileCAS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// Reuse the native service TestMain's isolated PG and the B14 query barrier.
	// The barrier pauses only the metadata SQL, after its actual old user read.
	cfg := testPool.Config()
	cfg.ConnConfig.Tracer = profileWriteTracer{}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	repo, roles := repository.NewUserRepo(pool), repository.NewRoleRepo(pool)
	users := service.NewUserService(pool, repo, roles, nil, nil, nil)
	actor := &model.User{Username: "avatar_manager", EmployeeNo: "E8AM" + uniqueSuffix(), Password: "hash", Status: 1}
	require.NoError(t, repo.Create(ctx, actor))
	role := &model.Role{Code: "avatar_manager_" + uniqueSuffix(), Name: "avatar manager", Priority: 10, Status: 1}
	require.NoError(t, roles.Create(ctx, role))
	require.NoError(t, repo.SetRoles(ctx, actor.ID, []int64{role.ID}))
	for _, writer := range []string{"self_profile", "admin_profile"} {
		for _, order := range []string{"bind_during_profile", "profile_then_bind", "bind_then_profile", "same_key_during_profile"} {
			t.Run(writer+"/"+order, func(t *testing.T) {
				target := &model.User{Username: "avatar_target", EmployeeNo: "E8AV" + uniqueSuffix(), Password: "hash", Status: 1}
				require.NoError(t, repo.Create(ctx, target))
				bucket := "avatar-cas-" + uniqueSuffix()
				oldKey, newKey := "avatars/old-"+uniqueSuffix()+".png", "avatars/new-"+uniqueSuffix()+".png"
				client := &avatarProfileClient{keys: map[string]bool{oldKey: true, newKey: true}}
				storageRepo := repository.NewStorageRepo(pool)
				for _, key := range []string{oldKey, newKey} {
					f := &model.FileObject{Bucket: bucket, ObjectKey: key, Purpose: "avatar", Status: model.FileStatusPending, FileName: "avatar.png", MIMEType: "image/png", CreatedBy: target.ID}
					require.NoError(t, storageRepo.InsertPending(ctx, f))
					require.NoError(t, storageRepo.Confirm(ctx, f.ID, 6, "avatar-etag"))
				}
				storage := storageservice.NewService(storageRepo, client, pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Enabled: true, Bucket: bucket}), slog.Default())
				_, err := storage.BindAvatar(ctx, oldKey, target.ID)
				require.NoError(t, err)
				current, err := repo.FindByID(ctx, target.ID)
				require.NoError(t, err)
				name := "profile edit"
				write := func(wctx context.Context) (*model.User, error) {
					if writer == "self_profile" {
						return users.UpdateProfile(wctx, target.ID, &model.UpdateProfileRequest{RealName: &name})
					}
					return users.Update(wctx, &model.UpdateUserRequest{ID: target.ID, Version: current.Version, RealName: &name}, actor.ID)
				}
				beforeVersion, beforeTime := current.Version, current.UpdatedAt
				var written *model.User
				var writeErr error
				if order == "bind_during_profile" || order == "same_key_during_profile" {
					gate := &profileWriteGate{make(chan struct{}), make(chan struct{})}
					var once sync.Once
					release := func() { once.Do(func() { close(gate.release) }) }
					defer release()
					type result struct {
						user *model.User
						err  error
					}
					done := make(chan result, 1)
					go func() { u, e := write(context.WithValue(ctx, profileWriteGateKey{}, gate)); done <- result{u, e} }()
					select {
					case <-gate.reached:
					case <-ctx.Done():
						t.Fatal("profile did not reach SQL barrier")
					}
					key := newKey
					if order == "same_key_during_profile" {
						key = oldKey
					}
					_, err = storage.BindAvatar(ctx, key, target.ID)
					require.NoError(t, err)
					if order == "same_key_during_profile" {
						bound, err := repo.FindByID(ctx, target.ID)
						require.NoError(t, err)
						require.Equal(t, beforeVersion, bound.Version)
						require.Equal(t, beforeTime, bound.UpdatedAt, "same-key binding changes no user metadata")
					}
					release()
					select {
					case r := <-done:
						written, writeErr = r.user, r.err
					case <-ctx.Done():
						t.Fatal("profile did not finish after release")
					}
				} else if order == "profile_then_bind" {
					written, writeErr = write(ctx)
					_, err = storage.BindAvatar(ctx, newKey, target.ID)
					require.NoError(t, err)
				} else {
					_, err = storage.BindAvatar(ctx, newKey, target.ID)
					require.NoError(t, err)
					current, err = repo.FindByID(ctx, target.ID)
					require.NoError(t, err)
					written, writeErr = write(ctx)
				}
				final, err := repo.FindByID(ctx, target.ID)
				require.NoError(t, err)
				old, err := storageRepo.GetByObjectKey(ctx, bucket, oldKey)
				require.NoError(t, err)
				t.Logf("writer=%s order=%s error=%v avatar=%s old_state=%s version=%d→%d", writer, order, writeErr, final.Avatar, old.Status, beforeVersion, final.Version)
				if order == "same_key_during_profile" {
					require.NoError(t, writeErr)
					require.Equal(t, oldKey, final.Avatar)
					require.Equal(t, beforeVersion+1, final.Version, "same-key binding must not spuriously stale the profile")
					require.Equal(t, model.FileStatusConfirmed, old.Status)
					_, err = client.StatObject(ctx, oldKey)
					require.NoError(t, err)
				} else {
					require.Equal(t, model.FileStatusDeleted, old.Status)
					_, oldStatErr := client.StatObject(ctx, oldKey)
					require.ErrorIs(t, oldStatErr, pkgstorage.ErrObjectNotFound)
					_, newStatErr := client.StatObject(ctx, newKey)
					require.NoError(t, newStatErr)
					t.Logf("retirement verified: old Stat=%v; new Stat=%v", oldStatErr, newStatErr)
					require.Equal(t, newKey, final.Avatar, "successful new avatar must not be replaced by a stale metadata snapshot")
					if order == "bind_during_profile" {
						require.ErrorIs(t, writeErr, errcode.ErrConcurrentModification)
						require.Equal(t, beforeVersion+1, final.Version)
						require.True(t, final.UpdatedAt.After(beforeTime), "new avatar advances metadata timestamp")
					} else {
						require.NoError(t, writeErr)
						require.Equal(t, name, final.RealName)
						require.NotNil(t, written)
					}
				}
			})
		}
	}
	// Legacy explicit URL edits and unrelated metadata continue to use the same
	// public profile contract; a normal bind is not made dependent on a version in
	// its request, and a rejected missing object cannot advance the user's version.
	t.Run("legacy_URL_and_rejected_bind", func(t *testing.T) {
		target := &model.User{Username: "avatar_url", EmployeeNo: "E8AU" + uniqueSuffix(), Password: "hash", Status: 1}
		require.NoError(t, repo.Create(ctx, target))
		url := "https://example.invalid/avatar.png"
		u, err := users.UpdateProfile(ctx, target.ID, &model.UpdateProfileRequest{Avatar: &url})
		require.NoError(t, err)
		require.Equal(t, url, u.Avatar)
		s := storageservice.NewService(repository.NewStorageRepo(pool), &avatarProfileClient{keys: map[string]bool{}}, pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Enabled: true, Bucket: "avatar-missing"}), slog.Default())
		_, err = s.BindAvatar(ctx, "avatars/missing.png", target.ID)
		require.ErrorIs(t, err, errcode.ErrFileNotFound)
		final, err := repo.FindByID(ctx, target.ID)
		require.NoError(t, err)
		require.Equal(t, u.Version, final.Version)
		require.Equal(t, url, final.Avatar)
	})
}

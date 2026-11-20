//go:build integration

package service_test

import (
	"context"
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
)

func TestC02ProfileInternalAvatar(t *testing.T) {
	ctx := trustedIAMTestContext()
	repo, roles := repository.NewUserRepo(testPool), repository.NewRoleRepo(testPool)
	svc := service.NewUserService(testPool, repo, roles, nil, nil, nil)
	actor := &model.User{Username: "manager", EmployeeNo: "E8C02" + uniqueSuffix(), Password: "hash", Status: 1}
	require.NoError(t, repo.Create(ctx, actor))
	role := &model.Role{Code: "c02_" + uniqueSuffix(), Name: "manager", Priority: 10, Status: 1}
	require.NoError(t, roles.Create(ctx, role))
	require.NoError(t, repo.SetRoles(ctx, actor.ID, []int64{role.ID}))
	for _, writer := range []string{"profile", "admin"} {
		for _, state := range []string{"foreign", "pending", "deleted", "wrong_purpose", "contradictory_owner", "valid", "external", "unchanged"} {
			t.Run(writer+"/"+state, func(t *testing.T) {
				target := &model.User{Username: "target", EmployeeNo: "E8C02" + uniqueSuffix(), Password: "hash", Status: 1}
				require.NoError(t, repo.Create(ctx, target))
				key := "avatars/" + uniqueSuffix() + ".png"
				bucket := "c02-" + uniqueSuffix()
				svc.SetStorageConfig(pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Enabled: true, Bucket: bucket}))
				creator := target.ID
				status, purpose := "confirmed", "avatar"
				if state == "foreign" {
					creator = actor.ID
				}
				if state == "pending" || state == "deleted" {
					status = state
				}
				if state == "wrong_purpose" {
					purpose = "attachment_create"
				}
				_, err := testPool.Exec(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by) VALUES($1,$2,$3,$4,'x','image/png',$5)`, bucket, key, purpose, status, creator)
				require.NoError(t, err)
				if state == "contradictory_owner" {
					_, err = testPool.Exec(ctx, `UPDATE file_objects SET owner_kind='user',owner_id=$3::bigint::text WHERE bucket=$1 AND object_key=$2`, bucket, key, actor.ID)
					require.NoError(t, err)
				}
				if state == "external" {
					key = "https://example.invalid/avatar.png"
				}
				if state == "unchanged" {
					_, err = testPool.Exec(ctx, `UPDATE users SET avatar=$2 WHERE id=$1`, target.ID, key)
					require.NoError(t, err)
					_, err = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, bucket)
					require.NoError(t, err)
				}
				name := "new name"
				var u *model.User
				if writer == "profile" {
					u, err = svc.UpdateProfile(ctx, target.ID, &model.UpdateProfileRequest{RealName: &name, Avatar: &key})
				} else {
					u, err = svc.Update(ctx, &model.UpdateUserRequest{ID: target.ID, Version: target.Version, RealName: &name, Avatar: &key}, actor.ID)
				}
				final, readErr := repo.FindByID(ctx, target.ID)
				require.NoError(t, readErr)
				valid := state == "valid" || state == "external"
				if !valid {
					require.ErrorIs(t, err, errcode.ErrFileNotFound, "unsafe explicit internal avatar must reject")
					if state == "unchanged" {
						require.Equal(t, key, final.Avatar)
					} else {
						require.Empty(t, final.Avatar)
					}
					require.Empty(t, final.RealName)
					require.Equal(t, target.Version, final.Version)
					return
				}
				require.NoError(t, err)
				require.Equal(t, key, u.Avatar)
				require.Equal(t, name, final.RealName)
				if state == "valid" {
					var owned bool
					require.NoError(t, testPool.QueryRow(ctx, `SELECT (owner_kind='user' AND owner_id=$3::bigint::text) IS TRUE FROM file_objects WHERE bucket=$1 AND object_key=$2`, bucket, key, target.ID).Scan(&owned))
					require.True(t, owned, "profile internal key must acquire target user's durable binding")
				}
			})
		}
	}
	t.Run("create_rejects_internal", func(t *testing.T) {
		_, err := svc.Create(ctx, &model.CreateUserRequest{Username: "new", EmployeeNo: "E8C02" + uniqueSuffix(), Password: "Synthetic123!", Avatar: "avatars/existing.png"}, actor.ID)
		require.ErrorIs(t, err, errcode.ErrFileNotFound)
	})
}

// Metadata writers must arbitrate on the object row with GC, rather than only
// checking object state before entering the profile transaction.
func TestC02ProfileAvatarBindingWinsGC(t *testing.T) {
	for _, writer := range []string{"profile", "admin"} {
		t.Run(writer, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(trustedIAMTestContext(), 15*time.Second)
			defer cancel()
			cfg := testPool.Config()
			cfg.ConnConfig.Tracer = profileWriteTracer{}
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, err)
			defer pool.Close()
			repo, roles := repository.NewUserRepo(pool), repository.NewRoleRepo(pool)
			svc := service.NewUserService(pool, repo, roles, nil, nil, nil)
			actor := &model.User{Username: "manager", EmployeeNo: "E8C02" + uniqueSuffix(), Password: "hash", Status: 1}
			require.NoError(t, repo.Create(ctx, actor))
			role := &model.Role{Code: "c02race_" + uniqueSuffix(), Name: "manager", Priority: 10, Status: 1}
			require.NoError(t, roles.Create(ctx, role))
			require.NoError(t, repo.SetRoles(ctx, actor.ID, []int64{role.ID}))
			target := &model.User{Username: "target", EmployeeNo: "E8C02" + uniqueSuffix(), Password: "hash", Status: 1}
			require.NoError(t, repo.Create(ctx, target))
			key, bucket := "avatars/"+uniqueSuffix()+".png", "c02race-"+uniqueSuffix()
			svc.SetStorageConfig(pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Enabled: true, Bucket: bucket}))
			var id int64
			require.NoError(t, pool.QueryRow(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by) VALUES($1,$2,'avatar','confirmed','x','image/png',$3) RETURNING id`, bucket, key, target.ID).Scan(&id))
			gate := &profileWriteGate{make(chan struct{}), make(chan struct{})}
			var once sync.Once
			release := func() { once.Do(func() { close(gate.release) }) }
			defer release()
			done := make(chan error, 1)
			go func() {
				wctx := context.WithValue(ctx, profileWriteGateKey{}, gate)
				if writer == "profile" {
					_, e := svc.UpdateProfile(wctx, target.ID, &model.UpdateProfileRequest{Avatar: &key})
					done <- e
				} else {
					_, e := svc.Update(wctx, &model.UpdateUserRequest{ID: target.ID, Version: target.Version, Avatar: &key}, actor.ID)
					done <- e
				}
			}()
			select {
			case <-gate.reached:
			case <-ctx.Done():
				t.Fatal("profile did not reach metadata write")
			}
			type outcome struct {
				marked bool
				err    error
			}
			gcDone := make(chan outcome, 1)
			go func() {
				m, e := repository.NewStorageRepo(testPool).MarkAvatarOrphanDeleted(ctx, id, key)
				gcDone <- outcome{m, e}
			}()
			var early *outcome
			require.Eventually(t, func() bool {
				select {
				case r := <-gcDone:
					early = &r
					return true
				default:
				}
				var n int
				err := testPool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE datname=current_database() AND wait_event_type='Lock' AND query LIKE '%file_objects%'`).Scan(&n)
				return err == nil && n > 0
			}, 3*time.Second, 5*time.Millisecond, "GC must reach the object lock or finish")
			release()
			require.NoError(t, <-done)
			var result outcome
			if early != nil {
				result = *early
			} else {
				result = <-gcDone
			}
			require.NoError(t, result.err)
			require.False(t, result.marked, "committed profile reference must win GC")
			var avatar, status string
			require.NoError(t, pool.QueryRow(ctx, `SELECT u.avatar,f.status FROM users u JOIN file_objects f ON f.object_key=u.avatar WHERE u.id=$1`, target.ID).Scan(&avatar, &status))
			require.Equal(t, key, avatar)
			require.Equal(t, "confirmed", status)
		})
	}
}

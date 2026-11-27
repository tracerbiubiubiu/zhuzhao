//go:build integration

package service_test

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

// Real handlers/services/repos: HTTP proxy spelling must not bypass ownership,
// and a service without enabled storage must not claim a database-only object.
func TestC02ReviewAvatarHandler(t *testing.T) {
	ctx := trustedIAMTestContext()
	repo, roles := repository.NewUserRepo(testPool), repository.NewRoleRepo(testPool)
	actor := &model.User{Username: "review_manager", EmployeeNo: "E8CR" + uniqueSuffix(), Password: "hash", Status: 1}
	require.NoError(t, repo.Create(ctx, actor))
	role := &model.Role{Code: "review_" + uniqueSuffix(), Name: "manager", Priority: 10, Status: 1}
	require.NoError(t, roles.Create(ctx, role))
	require.NoError(t, repo.SetRoles(ctx, actor.ID, []int64{role.ID}))
	for _, writer := range []string{"profile", "admin"} {
		for _, shape := range []string{"disabled", "old_bucket_only", "relative_foreign", "absolute_foreign", "escaped_foreign", "same_foreign", "same_missing", "valid", "relative_valid", "absolute_valid", "escaped_valid", "current_same_key_old_bucket", "external", "external_query"} {
			t.Run(writer+"/"+shape, func(t *testing.T) {
				target := &model.User{Username: "review_target", EmployeeNo: "E8CR" + uniqueSuffix(), Password: "hash", Status: 1}
				require.NoError(t, repo.Create(ctx, target))
				key, bucket := "avatars/"+uniqueSuffix()+".png", "review-"+uniqueSuffix()
				creator := actor.ID
				if shape == "disabled" || shape == "old_bucket_only" || strings.Contains(shape, "valid") || shape == "current_same_key_old_bucket" {
					creator = target.ID
				}
				_, err := testPool.Exec(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by) VALUES($1,$2,'avatar','confirmed','a','image/png',$3)`, bucket, key, creator)
				require.NoError(t, err)
				currentBucket := bucket
				if shape == "old_bucket_only" {
					currentBucket = "current-" + uniqueSuffix()
				}
				if shape == "current_same_key_old_bucket" {
					_, err = testPool.Exec(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by) VALUES($1,$2,'avatar','confirmed','a','image/png',$3)`, "old-"+bucket, key, actor.ID)
					require.NoError(t, err)
				}
				value := key
				if shape == "relative_foreign" || shape == "relative_valid" {
					value = "/api/v1/files/" + key
				}
				if shape == "absolute_foreign" || shape == "absolute_valid" {
					value = "https://app.example.invalid/api/v1/files/" + key
				}
				if shape == "escaped_valid" || shape == "escaped_foreign" {
					value = "https://app.example.invalid/api/v1/files/" + strings.Replace(key, "avatars/", "avatars%2F", 1)
				}
				if shape == "same_foreign" || shape == "same_missing" {
					_, err = testPool.Exec(ctx, `UPDATE users SET avatar=$2 WHERE id=$1`, target.ID, value)
					require.NoError(t, err)
				}
				if shape == "same_missing" {
					_, err = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, bucket)
					require.NoError(t, err)
				}
				if shape == "external" {
					value = "https://external.example.invalid/picture.png"
				}
				if shape == "external_query" {
					value = "https://external.example.invalid/picture.png?next=/api/v1/files/" + key
				}
				svc := service.NewUserService(testPool, repo, roles, nil, nil, nil)
				cfg := pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Enabled: true, Bucket: currentBucket})
				svc.SetStorageConfig(cfg)
				if shape == "disabled" {
					cfg.Store(&pkgstorage.StorageConfig{Enabled: false, Bucket: currentBucket})
				}
				h := handler.NewUserHandler(svc, nil)
				r := gin.New()
				id := target.ID
				if writer == "admin" {
					id = actor.ID
				}
				r.Use(func(c *gin.Context) { c.Set("userID", id); c.Request = c.Request.WithContext(ctx); c.Next() })
				path := "/api/v1/user/profile/update"
				r.POST(path, h.UpdateProfile)
				if writer == "admin" {
					path = "/api/v1/users/update"
					r.POST(path, h.Update)
				}
				body, _ := json.Marshal(map[string]any{"avatar": value, "real_name": "must rollback", "id": fmt.Sprint(target.ID), "version": target.Version})
				req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(body)))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				r.ServeHTTP(w, req)
				valid := strings.Contains(shape, "valid") || shape == "current_same_key_old_bucket" || strings.HasPrefix(shape, "external")
				if valid {
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					final, err := repo.FindByID(ctx, target.ID)
					require.NoError(t, err)
					want := key
					if strings.HasPrefix(shape, "external") {
						want = value
					}
					require.Equal(t, want, final.Avatar)
					require.Equal(t, "must rollback", final.RealName)
					if !strings.HasPrefix(shape, "external") {
						var owner string
						require.NoError(t, testPool.QueryRow(ctx, `SELECT owner_id FROM file_objects WHERE bucket=$1 AND object_key=$2`, currentBucket, key).Scan(&owner))
						require.Equal(t, fmt.Sprint(target.ID), owner)
					}
					return
				}
				wantCode := http.StatusNotFound
				if shape == "disabled" {
					wantCode = http.StatusInternalServerError
				}
				require.Equal(t, wantCode, w.Code, w.Body.String())
				final, err := repo.FindByID(ctx, target.ID)
				require.NoError(t, err)
				require.Empty(t, final.RealName)
				require.Equal(t, target.Version, final.Version)
			})
		}
	}
}

// The real attachment repository pauses with the object lock held, immediately
// before its uploaded_by FK insertion. Avatar writers must reject that pending
// attachment without waiting for it while holding the users row.
type c02ConfirmGate struct{ reached, release chan struct{} }
type c02ConfirmKey struct{}
type c02ConfirmTracer struct{ gate *c02ConfirmGate }

func (tr c02ConfirmTracer) TraceQueryStart(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryStartData) context.Context {
	if strings.Contains(d.SQL, "SELECT purpose FROM file_objects WHERE id=$1 FOR UPDATE") {
		return context.WithValue(ctx, c02ConfirmKey{}, true)
	}
	return ctx
}
func (tr c02ConfirmTracer) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, d pgx.TraceQueryEndData) {
	if ctx.Value(c02ConfirmKey{}) == true && d.Err == nil {
		close(tr.gate.reached)
		select {
		case <-tr.gate.release:
		case <-ctx.Done():
		}
	}
}
func TestC02ReviewAvatarConcurrentAttachmentConfirm(t *testing.T) {
	for _, writer := range []string{"profile", "admin", "repository_bind"} {
		t.Run(writer, func(t *testing.T) {
			ctx, cancel := context.WithTimeout(trustedIAMTestContext(), 10*time.Second)
			defer cancel()
			repo, roles := repository.NewUserRepo(testPool), repository.NewRoleRepo(testPool)
			target := &model.User{Username: "review_target", EmployeeNo: "E8CR" + uniqueSuffix(), Password: "hash", Status: 1}
			require.NoError(t, repo.Create(ctx, target))
			bucket, key := "concurrent-"+uniqueSuffix(), "tickets/"+uniqueSuffix()+".png"
			var tid, fid int64
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO ticket_types(code,name,states,transitions) VALUES($1,'test','["open"]','{}') RETURNING id`, "c02_"+uniqueSuffix()).Scan(&tid))
			// Use the newly inserted type without relying on production seed data.
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path) SELECT code,'avatar concurrency','open',$2,1,'root' FROM ticket_types WHERE id=$1 RETURNING id`, tid, target.ID).Scan(&tid))
			require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by) VALUES($1,$2,'attachment_create','pending','x','image/png',$3) RETURNING id`, bucket, key, target.ID).Scan(&fid))
			gate := &c02ConfirmGate{make(chan struct{}), make(chan struct{})}
			cfg := testPool.Config()
			cfg.ConnConfig.Tracer = c02ConfirmTracer{gate}
			pool, err := pgxpool.NewWithConfig(ctx, cfg)
			require.NoError(t, err)
			defer pool.Close()
			var once sync.Once
			release := func() { once.Do(func() { close(gate.release) }) }
			defer release()
			confirming := make(chan error, 1)
			go func() {
				confirming <- repository.NewStorageRepo(pool).ConfirmAndBind(ctx, fid, tid, target.ID, 10, "etag", 20)
			}()
			select {
			case <-gate.reached:
			case <-ctx.Done():
				t.Fatal("attachment confirmation did not reach object barrier")
			}
			svc := service.NewUserService(testPool, repo, roles, nil, nil, nil)
			svc.SetStorageConfig(pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Enabled: true, Bucket: bucket}))
			limited, stop := context.WithTimeout(ctx, time.Second)
			defer stop()
			if writer == "profile" {
				_, err = svc.UpdateProfile(limited, target.ID, &model.UpdateProfileRequest{Avatar: &key})
			} else if writer == "admin" {
				_, err = svc.Update(limited, &model.UpdateUserRequest{ID: target.ID, Version: target.Version, Avatar: &key}, target.ID)
			} else {
				_, err = repository.NewStorageRepo(testPool).BindAvatar(limited, bucket, key, target.ID)
			}
			release()
			require.NoError(t, <-confirming)
			if writer == "repository_bind" {
				require.ErrorIs(t, err, repository.ErrStorageObjectNotFound)
			} else {
				require.ErrorIs(t, err, errcode.ErrFileNotFound)
			}
			var n int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1 AND file_object_id=$2`, tid, fid).Scan(&n))
			require.Equal(t, 1, n)
			final, err := repo.FindByID(ctx, target.ID)
			require.NoError(t, err)
			require.Empty(t, final.Avatar)
			require.Equal(t, target.Version, final.Version)
		})
	}
}

// Surviving historical proxy values must still stream the actual object after
// another user replaces their raw-key reference and the active GC sweep runs.
func TestC02ReviewHistoricalProxyStillReadable(t *testing.T) {
	for _, writer := range []string{"profile", "admin", "bind"} {
		for _, shape := range []string{"relative", "absolute", "escaped"} {
			t.Run(writer+"/"+shape, func(t *testing.T) {
				ctx := trustedIAMTestContext()
				repo, roles := repository.NewUserRepo(testPool), repository.NewRoleRepo(testPool)
				target := &model.User{Username: "review_target", EmployeeNo: "E8CR" + uniqueSuffix(), Password: "hash", Status: 1}
				require.NoError(t, repo.Create(ctx, target))
				other := &model.User{Username: "historical", EmployeeNo: "E8CR" + uniqueSuffix(), Password: "hash", Status: 1}
				require.NoError(t, repo.Create(ctx, other))
				fixtureKey := uniqueSuffix()
				bucket, old, next := "readable-"+fixtureKey, "avatars/old-"+fixtureKey+".png", "avatars/next-"+fixtureKey+".png"
				for _, key := range []string{old, next} {
					_, err := testPool.Exec(ctx, `INSERT INTO file_objects(bucket,object_key,purpose,status,file_name,mime_type,created_by,confirmed_at) VALUES($1,$2,'avatar','confirmed','a','image/png',$3,NOW()-interval '30 days')`, bucket, key, target.ID)
					require.NoError(t, err)
				}
				cfg := pkgstorage.NewConfigStore(&pkgstorage.StorageConfig{Enabled: true, Bucket: bucket, OrphanGCDays: 7})
				client := &avatarProfileClient{keys: map[string]bool{old: true, next: true}}
				sr := repository.NewStorageRepo(testPool)
				storage := storageservice.NewService(sr, client, cfg, slog.Default())
				_, err := storage.BindAvatar(ctx, old, target.ID)
				require.NoError(t, err)
				historical := "/api/v1/files/" + old
				if shape == "absolute" {
					historical = "https://app.example.invalid" + historical + "?v=1#avatar"
				}
				if shape == "escaped" {
					historical = "https://app.example.invalid/api/v1/files/" + strings.Replace(old, "avatars/", "avatars%2F", 1)
				}
				_, err = testPool.Exec(ctx, `UPDATE users SET avatar=$2 WHERE id=$1`, other.ID, historical)
				require.NoError(t, err)
				users := service.NewUserService(testPool, repo, roles, nil, nil, nil)
				users.SetStorageConfig(cfg)
				current, err := repo.FindByID(ctx, target.ID)
				require.NoError(t, err)
				if writer == "profile" {
					_, err = users.UpdateProfile(ctx, target.ID, &model.UpdateProfileRequest{Avatar: &next})
				} else if writer == "admin" {
					_, err = users.Update(ctx, &model.UpdateUserRequest{ID: target.ID, Version: current.Version, Avatar: &next}, target.ID)
				} else {
					_, err = storage.BindAvatar(ctx, next, target.ID)
				}
				require.NoError(t, err)
				orphans, err := sr.ListConfirmedAvatarOrphans(ctx, bucket, time.Now(), 100)
				require.NoError(t, err)
				require.Empty(t, orphans)
				require.NoError(t, storageservice.NewGCJob(sr, client, cfg, slog.Default(), false).Handle(ctx, nil))
				h := handler.NewStorageHandler(storage, nil, nil)
				r := gin.New()
				r.Use(func(c *gin.Context) { c.Set("userID", other.ID); c.Next() })
				r.GET("/api/v1/files/*key", h.ProxyDownload)
				w := httptest.NewRecorder()
				storedURL, err := url.Parse(historical)
				require.NoError(t, err)
				r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, storedURL.RequestURI(), nil))
				require.Equal(t, http.StatusOK, w.Code, w.Body.String())
				require.Equal(t, "avatar", w.Body.String())
				final, err := repo.FindByID(ctx, target.ID)
				require.NoError(t, err)
				require.Equal(t, next, final.Avatar)
			})
		}
	}
}

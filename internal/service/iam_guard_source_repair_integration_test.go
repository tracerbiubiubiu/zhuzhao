//go:build integration

package service_test

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

func sourceHTTP(t *testing.T, actor int64, path string, self bool, action gin.HandlerFunc, body string) *httptest.ResponseRecorder {
	t.Helper()
	m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	e, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	_, err = e.AddPolicy("role::protect_actor", path, "POST")
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(), `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p','role::protect_actor',$1,'POST') ON CONFLICT DO NOTHING`, path)
	require.NoError(t, err)
	rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool), repository.NewMenuRepo(testPool), e)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("userID", actor); c.Next() })
	if self {
		r.Use(middleware.SelfService())
	}
	r.Use(middleware.CasbinAuth(e, rbac, nil))
	r.POST(path, action)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	return w
}

func TestIAMGuard_HTTPSourceOperationsCannotStripOrRestoreStrongerGrant(t *testing.T) {
	for _, qualified := range []bool{false, true} {
		for _, command := range []string{"remove_member", "pause_parent", "resume_parent", "move_to_paused", "move_from_paused", "delete"} {
			t.Run(fmt.Sprintf("%s/qualified_%t", command, qualified), func(t *testing.T) {
				f := targetProtectionSetup(t)
				ctx := context.Background()
				if qualified {
					_, err := testPool.Exec(ctx, `UPDATE roles SET priority=5 WHERE id IN(SELECT role_id FROM user_roles WHERE user_id=$1)`, f.actor)
					require.NoError(t, err)
				}
				var dest int64
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status) VALUES($1,'合成移动目的',1,$2::ltree,1) RETURNING id`, "source_dest_"+uniqueSuffix(), "root.dest_"+uniqueSuffix()).Scan(&dest))
				_, err := testPool.Exec(ctx, `INSERT INTO menus(code,name,menu_type,permission) VALUES('source_member','合成成员动作',3,'org:member'),('source_delete','合成删除动作',3,'org:delete')`)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, `INSERT INTO role_menus(role_id,menu_id) SELECT ur.role_id,m.id FROM user_roles ur,menus m WHERE ur.user_id=$1 AND m.code IN('source_member','source_delete')`, f.actor)
				require.NoError(t, err)
				if command == "resume_parent" || command == "move_from_paused" {
					_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, f.parent)
					require.NoError(t, err)
				}
				if command == "move_to_paused" {
					_, err = testPool.Exec(ctx, `UPDATE organizations SET status=0 WHERE id=$1`, dest)
					require.NoError(t, err)
				}
				if command == "delete" {
					_, err = testPool.Exec(ctx, `DELETE FROM user_orgs WHERE org_id=$1`, f.org)
					require.NoError(t, err)
				}
				var before string
				snapshot := func() string {
					var s string
					require.NoError(t, testPool.QueryRow(ctx, `SELECT md5(jsonb_agg(to_jsonb(o) ORDER BY id)::text) FROM organizations o WHERE id=ANY($1)`, []int64{f.parent, f.org, dest}).Scan(&s))
					return s
				}
				before = snapshot()
				targetBefore := targetProtectionSnapshot(t, f.target)
				h := handler.NewOrgHandler(grantOrgService(t))
				path := ""
				body := ""
				self := false
				var action gin.HandlerFunc
				switch command {
				case "remove_member":
					path = "/api/v1/orgs/members/delete"
					self = true
					action = h.RemoveMember
					body = fmt.Sprintf(`{"org_id":%q,"user_id":%q}`, strconv.FormatInt(f.org, 10), strconv.FormatInt(f.target, 10))
				case "pause_parent", "resume_parent":
					path = "/api/v1/orgs/update"
					action = h.Update
					status := 0
					if command == "resume_parent" {
						status = 1
					}
					body = fmt.Sprintf(`{"id":%q,"name":"合成保护父组","version":1,"status":%d}`, strconv.FormatInt(f.parent, 10), status)
				case "move_to_paused", "move_from_paused":
					path = "/api/v1/orgs/move"
					action = h.Move
					id := f.parent
					if command == "move_from_paused" {
						id = f.org
					}
					body = fmt.Sprintf(`{"id":%q,"new_parent_id":%q}`, strconv.FormatInt(id, 10), strconv.FormatInt(dest, 10))
				case "delete":
					path = "/api/v1/orgs/delete"
					self = true
					action = h.Delete
					body = fmt.Sprintf(`{"org_id":%q}`, strconv.FormatInt(f.org, 10))
				}
				w := sourceHTTP(t, f.actor, path, self, action, body)
				if qualified {
					if command == "delete" {
						require.Equal(t, http.StatusConflict, w.Code, w.Body.String())
						require.Contains(t, w.Body.String(), fmt.Sprintf(`"code":%d`, errcode.ErrOrgHasOrgRoles.Code))
						require.Equal(t, before, snapshot())
						require.NoError(t, repository.NewOrgRepo(testPool).UnbindOrgRole(trustedIAMTestContext(), f.org, f.strong.ID))
						w = sourceHTTP(t, f.actor, path, self, action, body)
					}
					require.Equal(t, http.StatusOK, w.Code, w.Body.String())
					return
				}
				require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
				require.Contains(t, w.Body.String(), fmt.Sprintf(`"code":%d`, errcode.ErrCannotAssignHigherRole.Code))
				require.Equal(t, before, snapshot())
				require.Equal(t, targetBefore, targetProtectionSnapshot(t, f.target))
			})
		}
	}
}

func TestIAMGuard_SourceProtectionKeepsDisplaySelfAndActualIDBoundaries(t *testing.T) {
	for _, kind := range []string{"display", "unchanged_status", "unchanged_parent", "self_leave", "reused_path_other_subtree"} {
		t.Run(kind, func(t *testing.T) {
			f := targetProtectionSetup(t)
			ctx := context.Background()
			h := handler.NewOrgHandler(grantOrgService(t))
			path := "/api/v1/orgs/update"
			self := false
			action := h.Update
			body := fmt.Sprintf(`{"id":%q,"name":"Synthetic display","version":1}`, strconv.FormatInt(f.parent, 10))
			switch kind {
			case "unchanged_status":
				body = fmt.Sprintf(`{"id":%q,"name":"Synthetic display","version":1,"status":1}`, strconv.FormatInt(f.parent, 10))
			case "unchanged_parent":
				path = "/api/v1/orgs/move"
				action = h.Move
				body = fmt.Sprintf(`{"id":%q,"new_parent_id":%q}`, strconv.FormatInt(f.org, 10), strconv.FormatInt(f.parent, 10))
			case "self_leave":
				_, err := testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, f.actor, f.org)
				require.NoError(t, err)
				_, err = testPool.Exec(ctx, `UPDATE organizations SET owner_user_ids=ARRAY[$1::bigint] WHERE id=$2`, f.actor, f.org)
				require.NoError(t, err)
				path = "/api/v1/orgs/members/delete"
				self = true
				action = h.RemoveMember
				body = fmt.Sprintf(`{"org_id":%q,"user_id":%q}`, strconv.FormatInt(f.org, 10), strconv.FormatInt(f.actor, 10))
			case "reused_path_other_subtree":
				// A historical same-path tree is a different identity. Its surviving
				// child must not be attributed to the new parent's source operation.
				_, err := testPool.Exec(ctx, `DELETE FROM org_roles WHERE org_id=$1`, f.org)
				require.NoError(t, err)
				var old, child int64
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status,deleted_at) SELECT code,'old root',1,path,1,NOW() FROM organizations WHERE id=$1 RETURNING id`, f.parent).Scan(&old))
				require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,parent_id,path,status) SELECT $1::text,'old child',$2,(path::text||'.historical_child')::ltree,1 FROM organizations WHERE id=$2 RETURNING id`, "source_history_"+uniqueSuffix(), old).Scan(&child))
				_, err = testPool.Exec(ctx, `INSERT INTO org_roles(org_id,role_id) VALUES($1,$2)`, child, f.strong.ID)
				require.NoError(t, err)
				body = fmt.Sprintf(`{"id":%q,"name":"Synthetic display","version":1,"status":0}`, strconv.FormatInt(f.parent, 10))
			}
			w := sourceHTTP(t, f.actor, path, self, action, body)
			require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		})
	}
}

//go:build integration

package handler_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/gin-gonic/gin"
	goredis "github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"

	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
)

type notificationPrefFixture struct {
	router *gin.Engine
	a, b   int64
	at, bt string
}

// newNotificationPrefFixture uses the real authentication injection contract:
// setting an extra user_id alias here would conceal either handler regression.
func newNotificationPrefFixture(t *testing.T) notificationPrefFixture {
	t.Helper()
	ctx := context.Background()
	suffix := fmt.Sprintf("pref_%d", time.Now().UnixNano())
	f := notificationPrefFixture{}
	var roleID int64
	for i, id := range []*int64{&f.a, &f.b} {
		name := fmt.Sprintf("%s_%d", suffix, i)
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO users(username,password,status) VALUES($1,'synthetic',1) RETURNING id`, name).Scan(id))
	}
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,status,priority) VALUES($1,$1,1,30) RETURNING id`, suffix).Scan(&roleID))
	_, err := testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$3),($2,$3)`, f.a, f.b, roleID)
	require.NoError(t, err)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM notification_optouts WHERE user_id IN ($1,$2)`, f.a, f.b)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM user_roles WHERE role_id=$1`, roleID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM roles WHERE id=$1`, roleID)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id IN ($1,$2)`, f.a, f.b)
	})
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	manager := jwtpkg.NewManager(jwtpkg.Config{Secret: "notification-pref-test-0123456789abcdef", AccessTTL: time.Hour})
	f.at, _, err = manager.GenerateAccessToken(f.a, suffix+"_a", false)
	require.NoError(t, err)
	f.bt, _, err = manager.GenerateAccessToken(f.b, suffix+"_b", false)
	require.NoError(t, err)
	rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool), repository.NewMenuRepo(testPool), nil)
	h := handler.NewNotificationPrefHandler(service.NewNotificationPrefService(testPool))
	f.router = gin.New()
	f.router.Use(middleware.RequestID(), middleware.JWT(manager, rdb, nil), middleware.SelfService(), middleware.CasbinAuth(nil, rbac, nil))
	f.router.GET("/api/v1/me/notification-optouts", h.ListOptouts)
	f.router.POST("/api/v1/me/notification-optouts", h.SetOptout)
	return f
}

func (f notificationPrefFixture) request(method, token, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/api/v1/me/notification-optouts", strings.NewReader(body))
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	f.router.ServeHTTP(w, req)
	return w
}

func prefRows(t *testing.T, uid int64) []service.OptoutEntry {
	t.Helper()
	out, err := service.NewNotificationPrefService(testPool).ListOptouts(context.Background(), uid)
	require.NoError(t, err)
	return out
}

// TestNotificationPrefsListAuthenticatedIdentity guards the GET context key
// through real JWT, role lookup and PG; a directly seeded row prevents false empty success.
func TestNotificationPrefsListAuthenticatedIdentity(t *testing.T) {
	f := newNotificationPrefFixture(t)
	_, err := testPool.Exec(context.Background(), `INSERT INTO notification_optouts(user_id,biz_type,event_action) VALUES($1,'ticket','ticket.closed'),($2,'ticket','ticket.assigned')`, f.a, f.b)
	require.NoError(t, err)
	require.Equal(t, http.StatusUnauthorized, f.request(http.MethodGet, "", "").Code)
	for _, tc := range []struct{ token, action string }{{f.at, "ticket.closed"}, {f.bt, "ticket.assigned"}} {
		w := f.request(http.MethodGet, tc.token, "")
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var envelope struct {
			Code int                   `json:"code"`
			Data []service.OptoutEntry `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
		require.Zero(t, envelope.Code)
		require.Equal(t, []service.OptoutEntry{{BizType: "ticket", EventAction: tc.action}}, envelope.Data)
	}
}

// TestNotificationPrefsSetAuthenticatedIdentity guards both enabling a preference
// and clearing its persisted row, so a false-success DELETE cannot pass.
func TestNotificationPrefsSetAuthenticatedIdentity(t *testing.T) {
	f := newNotificationPrefFixture(t)
	body := fmt.Sprintf(`{"biz_type":"ticket","event_action":"ticket.closed","opt_out":true,"user_id":"%d"}`, f.b)
	require.Equal(t, http.StatusUnauthorized, f.request(http.MethodPost, "", body).Code)
	require.Empty(t, prefRows(t, f.a))
	require.Empty(t, prefRows(t, f.b))
	for _, token := range []string{f.at, f.at, f.bt} {
		w := f.request(http.MethodPost, token, body)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Contains(t, w.Body.String(), `"code":0`)
		require.Equal(t, []service.OptoutEntry{{BizType: "ticket", EventAction: "ticket.closed"}}, prefRows(t, f.a))
	}
	require.Len(t, prefRows(t, f.b), 1)
	for range 2 {
		w := f.request(http.MethodPost, f.at, `{"biz_type":"ticket","event_action":"ticket.closed","opt_out":false}`)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		require.Empty(t, prefRows(t, f.a))
		require.Len(t, prefRows(t, f.b), 1)
	}
}

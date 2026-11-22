//go:build integration

package service_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

func TestTicketRelationCandidatesMatchBothUpdatePermissions(t *testing.T) {
	ctx := context.Background()
	code := "rel_" + uniqueSuffix()
	keyword := strings.Map(func(r rune) rune {
		if r >= '0' && r <= '9' {
			return 'a' + r - '0'
		}
		return r
	}, code)

	actor := rbacTestUser(t, code, "RE"+uniqueSuffix(), nil)
	other := rbacTestUser(t, code+"_peer", "RE"+uniqueSuffix(), nil)
	var roleID, delegatedOrg, ordinaryOrg int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO roles(code,name,priority)VALUES($1::text,$1::text,50) RETURNING id`, code).Scan(&roleID))
	_, err := testPool.Exec(ctx, `INSERT INTO user_roles(user_id,role_id) VALUES($1,$2)`, actor, roleID)
	require.NoError(t, err)
	for i, org := range []*int64{&delegatedOrg, &ordinaryOrg} {
		orgCode := fmt.Sprintf("%s_%d", code, i)
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO organizations(code,name,path,ticket_visibility) VALUES($1::text,$1::text,$1::text::ltree,'project_isolated') RETURNING id`, orgCode).Scan(org))
	}
	_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,org_member_role) VALUES($1,$2,'admin')`, actor, delegatedOrg)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO ticket_types(code,name) VALUES($1::text,$1::text)`, code)
	require.NoError(t, err)
	definition := publishC04FixtureDefinition(t, ctx, code, actor, nil)
	insert := func(creator, orgID int64, assignee *int64) int64 {
		t.Helper()
		var id int64
		require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,created_by,org_id,org_path,assigned_to,definition_version)
			SELECT $1::text,$5,'open',$2,$3,path,$4,$6 FROM organizations WHERE id=$3 RETURNING id`, code, creator, orgID, assignee, keyword, definition).Scan(&id))
		return id
	}
	source := insert(actor, ordinaryOrg, nil)
	own := insert(actor, ordinaryOrg, nil)
	delegated := insert(other, delegatedOrg, nil)
	readOnly := insert(other, ordinaryOrg, &actor)
	hidden := insert(other, ordinaryOrg, nil)
	for range 21 {
		insert(other, ordinaryOrg, &actor)
	}

	const path, relationPath = "/api/v1/tickets", "/api/v1/tickets/relations"
	m, err := casbinmodel.NewModelFromFile("../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	_, err = enforcer.AddPolicy("role::"+code, path, "GET")
	require.NoError(t, err)
	_, err = enforcer.AddPolicy("role::"+code, relationPath, "POST")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST'), ('p',$1,$3,'GET')`, "role::"+code, relationPath, path)
	require.NoError(t, err)
	rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool), repository.NewMenuRepo(testPool), enforcer)
	svc := ticketsvc.NewTicketService(testPool, repository.NewTicketRepo(testPool), repository.NewOrgRepo(testPool), resource.NewRegistry(), rbac, service.NewOrgDelegationService(testPool), nil, nil, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", actor)
		c.Set("session_epoch", int64(1)) // 初始数据库安全代际的合成准入事实
		c.Next()
	}, middleware.CasbinAuth(enforcer, rbac, nil))
	h := handler.NewTicketHandler(svc, nil, nil)
	r.GET(path, h.List)
	r.POST(relationPath, h.CreateRelation)
	write := func(from, to int64) *httptest.ResponseRecorder {
		t.Helper()
		w := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, relationPath,
			bytes.NewBufferString(fmt.Sprintf(`{"source_ticket_id":"%d","target_ticket_id":"%d"}`, from, to)))
		req.Header.Set("Content-Type", "application/json")
		r.ServeHTTP(w, req)
		return w
	}
	query := func(from int64, keyword string) *httptest.ResponseRecorder {
		t.Helper()
		values := url.Values{"relation_source_id": {fmt.Sprint(from)}, "keyword": {keyword}}
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path+"?"+values.Encode(), nil))
		return w
	}
	w := query(source, keyword)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 2, "permission filtering precedes the twenty-row limit")
	require.Equal(t, fmt.Sprint(delegated), resp.Data[0].ID)
	require.Equal(t, fmt.Sprint(own), resp.Data[1].ID)
	require.NotContains(t, w.Body.String(), "custom_data")
	w = query(source, fmt.Sprint(own))
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Data, 1)
	require.Equal(t, fmt.Sprint(own), resp.Data[0].ID)
	require.Equal(t, http.StatusForbidden, query(readOnly, keyword).Code)
	require.Equal(t, http.StatusNotFound, query(hidden, keyword).Code)
	asserted := write(readOnly, 1<<61)
	require.Equal(t, http.StatusForbidden, asserted.Code, "the source rejection precedes any target lookup: %s", asserted.Body)
	require.Equal(t, http.StatusForbidden, write(readOnly, hidden).Code)
	require.Equal(t, http.StatusOK, write(source, own).Code)
	require.Equal(t, http.StatusConflict, write(own, source).Code)

	_, err = testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2 AND v2='POST'`, "role::"+code, relationPath)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, query(source, keyword).Code)
	_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,'POST')`, "role::"+code, relationPath)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `DELETE FROM casbin_rule WHERE v0=$1 AND v1=$2 AND v2='GET'`, "role::"+code, path)
	require.NoError(t, err)
	require.Equal(t, http.StatusForbidden, query(source, keyword).Code)

}

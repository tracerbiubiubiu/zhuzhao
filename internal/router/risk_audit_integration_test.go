//go:build integration

package router_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	jwtpkg "github.com/tracerbiubiubiu/zhuzhao-utils/jwt"
	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/router"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service/risk"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

func TestRiskRoutes_PostgresAuditMetadataAndHistoricalReaderProjection(t *testing.T) {
	gin.SetMode(gin.TestMode)
	db, _, err := testutil.SetupPostgresShared()
	require.NoError(t, err)
	ctx := context.Background()
	enforcer := newRouterTestEnforcer(t)
	const listPath = "/api/v1/risk-items"
	const auditPath = "/api/v1/audit/logs"
	const managerRole = "synthetic_risk_audit_manager"
	const auditorRole = "synthetic_risk_auditor"
	addActor := func(role string) int64 {
		t.Helper()
		var id int64
		require.NoError(t, db.QueryRow(ctx, `INSERT INTO users(username,password,employee_no,real_name,status,session_epoch,session_projection_epoch)
			VALUES($1,'synthetic-only',$1,$1,1,3,3) RETURNING id`, role).Scan(&id))
		_, insertErr := db.Exec(ctx, `WITH new_role AS (INSERT INTO roles(code,name,priority) VALUES($1,$1,60) RETURNING id)
			INSERT INTO user_roles(user_id,role_id) SELECT $2,id FROM new_role`, role, id)
		require.NoError(t, insertErr)
		return id
	}
	grant := func(role, path, method string) {
		t.Helper()
		_, addErr := enforcer.AddPolicy("role::"+role, path, method)
		require.NoError(t, addErr)
		_, insertErr := db.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3)`, "role::"+role, path, method)
		require.NoError(t, insertErr)
	}
	manager, auditor := addActor(managerRole), addActor(auditorRole)
	for _, path := range []string{"/create", "/assess", "/assign"} {
		grant(managerRole, listPath+path, http.MethodPost)
	}
	grant(managerRole, listPath, http.MethodGet)
	grant(auditorRole, auditPath, http.MethodGet)
	var org int64
	require.NoError(t, db.QueryRow(ctx, `INSERT INTO organizations(code,name,path,status,is_virtual)
		VALUES('synthetic_risk_audit_org','synthetic risk audit','synthetic_risk_audit_org',1,true) RETURNING id`).Scan(&org))
	_, err = db.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id,org_member_role,ticket_scope)
		VALUES($1,$2,'admin','group')`, manager, org)
	require.NoError(t, err)

	m := jwtpkg.NewManager(jwtpkg.Config{Secret: "synthetic-risk-audit-test-identity", AccessTTL: time.Minute})
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	audit := service.NewAuditService(repository.NewAuditLogRepo(db), repository.NewUserRepo(db))
	r := router.New(router.Deps{
		DBPool: db, JWTManager: m, Enforcer: enforcer, RedisClient: startStubRedis(t), Logger: logger,
		RoleFetcher:  service.NewRBACService(repository.NewRoleRepo(db), repository.NewUserRepo(db), repository.NewMenuRepo(db), enforcer),
		AuditService: audit, AuditHandler: handler.NewAuditHandler(audit), RiskHandler: handler.NewRiskHandler(risk.NewService(db, logger), nil),
	})

	const createPath = listPath + "/create"
	create := fmt.Sprintf(`{"operation_id":"synthetic-create","org_id":"%d","title":"%s","description":"%s"}`, org, auditFixtureMarker, auditFixtureMarker)
	result := sendRiskAuditFixtureRequest(t, r, m, manager, http.MethodPost, createPath, create)
	require.Equal(t, http.StatusOK, result.Code, result.Body.String())
	var created struct{ Data model.RiskMutationResult }
	require.NoError(t, json.Unmarshal(result.Body.Bytes(), &created))
	require.Positive(t, created.Data.Risk.ID)
	riskID := created.Data.Risk.ID
	assertAuditRow := func(requestID, path string, actor int64, status int) {
		t.Helper()
		var actualPath, body, summary string
		var actualStatus int
		var actualActor int64
		require.NoError(t, db.QueryRow(ctx, `SELECT path, user_id, status_code, COALESCE(request_body,''), COALESCE(response_summary,'')
			FROM audit_logs WHERE request_id=$1`, requestID).Scan(&actualPath, &actualActor, &actualStatus, &body, &summary))
		assert.Equal(t, path, actualPath)
		assert.Equal(t, actor, actualActor)
		assert.Equal(t, status, actualStatus)
		assert.Empty(t, body)
		assert.Empty(t, summary)
	}
	assertAuditRow(result.Header().Get("X-Request-ID"), createPath, manager, http.StatusOK)
	for _, tc := range []struct{ path, body string }{
		{listPath + "/assess", fmt.Sprintf(`{"operation_id":"synthetic-assess","id":"%d","expected_revision":"1","level":"high","treatment":"mitigate","assessment_note":"%s"}`, riskID, auditFixtureMarker)},
		{listPath + "/assign", fmt.Sprintf(`{"operation_id":"synthetic-assign","id":"%d","expected_revision":"2","owner_user_id":"%d","reason":"%s"}`, riskID, manager, auditFixtureMarker)},
	} {
		response := sendRiskAuditFixtureRequest(t, r, m, manager, http.MethodPost, tc.path, tc.body)
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		assertAuditRow(response.Header().Get("X-Request-ID"), tc.path, manager, http.StatusOK)
	}
	list := sendRiskAuditFixtureRequest(t, r, m, manager, http.MethodGet, listPath+"?q="+auditFixtureMarker, "")
	require.Equal(t, http.StatusOK, list.Code, list.Body.String())
	require.Contains(t, list.Body.String(), auditFixtureMarker)
	assertAuditRow(list.Header().Get("X-Request-ID"), listPath, manager, http.StatusOK)
	denied := sendRiskAuditFixtureRequest(t, r, m, auditor, http.MethodPost, createPath, create)
	require.Equal(t, http.StatusForbidden, denied.Code, denied.Body.String())
	assertAuditRow(denied.Header().Get("X-Request-ID"), createPath, auditor, http.StatusForbidden)

	// An earlier version could leave protected text in an audit row. Audit permission
	// gives the auditor the row and outcome without making it a risk-object permission.
	_, err = db.Exec(ctx, `INSERT INTO audit_logs (method,path,status_code,duration,request_body,response_summary)
		VALUES ('POST',$1,200,0,$2,$2)`, createPath, auditFixtureMarker)
	require.NoError(t, err)
	logs := sendRiskAuditFixtureRequest(t, r, m, auditor, http.MethodGet, auditPath+"?path="+createPath, "")
	require.Equal(t, http.StatusOK, logs.Code, logs.Body.String())
	var auditList struct{ Data model.AuditLogListResponse }
	require.NoError(t, json.Unmarshal(logs.Body.Bytes(), &auditList))
	require.Len(t, auditList.Data.List, 3)
	assert.NotContains(t, logs.Body.String(), auditFixtureMarker)
}

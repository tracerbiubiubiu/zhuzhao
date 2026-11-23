//go:build integration

package storage_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// Exercise actual JSON handlers and IAM middleware: the assigned noncreator
// gains only workflow-evidence upload, while ordinary ticket update still fails.
func TestRiskWorkflowUploadRealHandler(t *testing.T) {
	ctx := context.Background()
	actor, _, roleCode := attachmentCommandFixture(t)
	creator := insertUser(t)
	client := &revokingStatClient{stubClient: stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "workflow-evidence", ContentType: "image/png"}}}
	b := newGuardBundle(t, testPool, client)
	typeCode := "workflow_upload_" + uniqueSuffix()
	_, err := b.ticket.CreateTicketType(ctx, &model.CreateTicketTypeRequest{Code: typeCode, Name: typeCode})
	require.NoError(t, err)
	draft, err := b.ticket.UpdateTicketType(ctx, typeCode, &model.UpdateTicketTypeRequest{
		Transitions:    json.RawMessage(`{"open":["assigned"],"assigned":["in_progress"],"in_progress":["pending_verify"],"pending_verify":["pending_verify","closed","in_progress"],"closed":["open"]}`),
		TransitionMeta: json.RawMessage(`{"in_progress>pending_verify":{"roles":["assignee"]}}`),
	})
	require.NoError(t, err)
	definition, changed, err := b.ticket.PublishTicketType(ctx, draft, creator)
	require.NoError(t, err)
	require.True(t, changed, "fixture must use a real published Gate-on definition")
	var orgID, ticketID int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM organizations WHERE code='root'`).Scan(&orgID))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO tickets(type_code,title,status,org_id,org_path,created_by,assigned_to,definition_version)
		SELECT $1,$1,'in_progress',id,path,$3,$4,$5 FROM organizations WHERE id=$2 RETURNING id`, typeCode, orgID, creator, actor, definition.DefinitionVersion).Scan(&ticketID))
	var groupID, familyID, riskID int64
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_dispatch_groups(finding_key,owner_user_id,management_org_id,target_org_id,type_code,definition_version,approved_scope,group_key,ticket_id,ticket_ref_snapshot,operator_user_id)
		VALUES($1::text,$2,$3,$3,$1::text,$4,'synthetic',$5,$6,$1::text,$2) RETURNING id`, typeCode, creator, orgID, definition.DefinitionVersion, strings.Repeat("a", 64), ticketID).Scan(&groupID))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_families(fingerprint_scope,canonical_key_hash) VALUES('synthetic',$1) RETURNING id`, typeCode).Scan(&familyID))
	require.NoError(t, testPool.QueryRow(ctx, `INSERT INTO risk_items(human_key,family_id,status,org_id,owner_user_id,title,created_by)
		VALUES($1,$2,'remediating',$3,$4,$1,$4) RETURNING id`, typeCode, familyID, orgID, creator).Scan(&riskID))
	_, err = testPool.Exec(ctx, `INSERT INTO risk_ticket_links(risk_id,group_id,owner_at_link) VALUES($1,$2,$3)`, riskID, groupID, creator)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `INSERT INTO user_orgs(user_id,org_id) VALUES($1,$2)`, actor, orgID)
	require.NoError(t, err)
	for _, route := range []struct{ path, method string }{
		{"/api/v1/risk-items/remediation-finish", http.MethodPost},
		{"/api/v1/tickets/transition", http.MethodPost},
		{"/api/v1/tickets/" + strconv.FormatInt(ticketID, 10), http.MethodGet},
	} {
		_, err = testPool.Exec(ctx, `INSERT INTO casbin_rule(ptype,v0,v1,v2) VALUES('p',$1,$2,$3)`, "role::"+roleCode, route.path, route.method)
		require.NoError(t, err)
	}
	ordinaryErr := b.ticket.AuthorizeTicketAttachment(ctx, actor, ticketID)
	require.Error(t, ordinaryErr)
	require.Equal(t, errcode.ErrNoPermission.Code, codeOf(t, ordinaryErr))

	m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool), repository.NewMenuRepo(testPool), enforcer)
	h := handler.NewStorageHandler(b.storage, ticketsvc.NewRiskWorkflowAttachmentCommand(b.ticket, true), nil)
	var epoch int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT session_epoch FROM users WHERE id=$1`, actor).Scan(&epoch))
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", actor)
		c.Set("session_epoch", epoch)
		c.Set("request_id", "workflow-upload-handler")
		c.Next()
	}, middleware.SelfService(), middleware.CasbinAuth(enforcer, rbac, testLogger()))
	r.POST("/api/v1/storage/presign/upload", h.PresignUpload)
	r.POST("/api/v1/tickets/attachments/confirm", h.Confirm)
	post := func(path, body string, out any) {
		t.Helper()
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code, w.Body.String())
		var envelope struct {
			Code int             `json:"code"`
			Data json.RawMessage `json:"data"`
		}
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &envelope))
		require.Zero(t, envelope.Code)
		require.NoError(t, json.Unmarshal(envelope.Data, out))
	}
	var presigned model.PresignUploadResponse
	post("/api/v1/storage/presign/upload", fmt.Sprintf(`{"purpose":"ticket_attachment","ticket_id":"%d","file_name":"evidence.png","mime_type":"image/png","size_bytes":"10"}`, ticketID), &presigned)
	require.NotEmpty(t, presigned.UploadURL)
	require.Equal(t, model.FileStatusPending, objectStatus(t, presigned.ObjectKey))
	var confirmed model.FileObject
	post("/api/v1/tickets/attachments/confirm", fmt.Sprintf(`{"ticket_id":"%d","object_key":%q}`, ticketID, presigned.ObjectKey), &confirmed)
	require.Equal(t, model.FileStatusConfirmed, confirmed.Status)
	require.Equal(t, actor, confirmed.CreatedBy)
	require.Equal(t, 1, attachmentCount(t, ticketID))
	require.False(t, client.headGuardActive, "object HEAD must run outside the IAM guard")
	var uploadedBy int64
	require.NoError(t, testPool.QueryRow(ctx, `SELECT uploaded_by FROM ticket_attachments WHERE ticket_id=$1 AND file_object_id=$2`, ticketID, confirmed.ID).Scan(&uploadedBy))
	require.Equal(t, actor, uploadedBy)
	var status string
	require.NoError(t, testPool.QueryRow(ctx, `SELECT status FROM tickets WHERE id=$1`, ticketID).Scan(&status))
	require.Equal(t, "in_progress", status, "upload itself must not finish remediation")
	require.Error(t, b.ticket.AuthorizeTicketAttachment(ctx, actor, ticketID), "successful evidence upload must not widen ordinary update")
}

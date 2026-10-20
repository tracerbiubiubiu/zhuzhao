//go:build integration

// B5-1 批6b-2：storage handler 契约测试——测试先行（先红后绿）。
// 契约 = 05 号 A.4：信封 {code,message,data,request_id}；91001/91002→400；
// 91003→404 防枚举同形（非本人与不存在逐字段同形）；91004→409。
// 路由挂法复刻 router 的 biz 组形态（登录态自服务面——鉴权链属中间件域，此处测 handler 契约）。
package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// denyAuth TicketAuthorizer 桩：deny 恒 403 / allow 恒过。
type denyAuth struct{ allow bool }

func (d *denyAuth) AuthorizeTicketAttachment(_ context.Context, _, _ int64) error {
	if d.allow {
		return nil
	}
	return errcode.ErrNoPermission
}

func (d *denyAuth) AuthorizeTicketFileRead(_ context.Context, _, _ int64) error {
	if d.allow {
		return nil
	}
	return errcode.ErrNoPermission
}

type hStubClient struct {
	stat    *pkgstorage.ObjectInfo
	getBody []byte
}

func (s *hStubClient) PresignPut(_ context.Context, key, contentType string, ttl time.Duration) (*pkgstorage.PresignedPut, error) {
	return &pkgstorage.PresignedPut{
		URL: "http://stub/" + key, ExpiresAt: time.Now().Add(ttl),
		Headers: map[string]string{"Content-Type": contentType},
	}, nil
}
func (s *hStubClient) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "http://stub/get/" + key, nil
}
func (s *hStubClient) StatObject(_ context.Context, _ string) (*pkgstorage.ObjectInfo, error) {
	return s.stat, nil
}

func (s *hStubClient) GetObject(_ context.Context, _ string, matchETag string) (io.ReadCloser, *pkgstorage.ObjectInfo, error) {
	if matchETag != "" && matchETag != s.stat.ETag {
		return nil, nil, pkgstorage.ErrObjectReplaced
	}
	return io.NopCloser(bytes.NewReader(s.getBody)), s.stat, nil
}
func (s *hStubClient) RemoveObject(_ context.Context, _ string) error { return nil }

// auditRecorder 审计桩（A.4 #5 收窄面断言用——S5 评审 P2-5 健壮性批）。
type auditRecorder struct{ entries []middleware.AuditLogEntry }

func (a *auditRecorder) Insert(_ context.Context, e middleware.AuditLogEntry) error {
	a.entries = append(a.entries, e)
	return nil
}

// insertHTicket 直插最小工单行（handler 包独立环境——storage 包 insertTicket 跨包不可用）。
func insertHTicket(t *testing.T, uid int64) int64 {
	t.Helper()
	ctx := context.Background()
	_, err := testPool.Exec(ctx, `
		INSERT INTO organizations (code, name, path, is_virtual, status, sort_order, is_system)
		SELECT 'root', 'Root', 'root', false, 1, 0, true
		WHERE NOT EXISTS (SELECT 1 FROM organizations WHERE code = 'root')`)
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, `
		INSERT INTO ticket_types (code, name, states, transitions)
		SELECT 'incident', '事件', '["open","assigned","closed"]', '{}'
		WHERE NOT EXISTS (SELECT 1 FROM ticket_types WHERE code = 'incident')`)
	require.NoError(t, err)
	var orgID int64
	require.NoError(t, testPool.QueryRow(ctx,
		`SELECT id FROM organizations WHERE code = 'root' LIMIT 1`).Scan(&orgID))
	var id int64
	require.NoError(t, testPool.QueryRow(ctx, `
		INSERT INTO tickets (type_code, title, description, priority, status, created_by, org_id, org_path)
		VALUES ('incident', 'h 附件审计测试', 'x', 3, 'open', $1, $2,
			(SELECT path FROM organizations WHERE id = $2))
		RETURNING id`, uid, orgID).Scan(&id))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM tickets WHERE id = $1`, id)
	})
	return id
}

// newStorageRouter 复刻 biz 组挂法；userID 经组中间件注入（鉴权链等价物）。
func newStorageRouter(t *testing.T) (*gin.Engine, int64) {
	return newStorageRouterWith(t, nil, false)
}

// newStorageRouterWith 参数化装配：审计桩注入 + TicketAuthorizer 放行开关
// （ticket 态 200 全链场景用——S5 评审 P2-5 健壮性批）。
func newStorageRouterWith(t *testing.T, audit middleware.AuditLogger, allowAuth bool) (*gin.Engine, int64) {
	t.Helper()
	var uid int64
	eno := fmt.Sprintf("EHSTG%d", time.Now().UnixNano())
	require.NoError(t, testPool.QueryRow(context.Background(),
		`INSERT INTO users (employee_no, username, password, status) VALUES ($1, $1, 'hash', 1) RETURNING id`, eno,
	).Scan(&uid))
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = 'it-h'`)
		_, _ = testPool.Exec(context.Background(), `DELETE FROM users WHERE id = $1`, uid)
	})

	cfg := &pkgstorage.StorageConfig{
		Enabled: true, Endpoint: "http://stub", Bucket: "it-h",
		AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m",
	}
	cfg.ApplyDefaults()
	svc := storageservice.NewService(repository.NewStorageRepo(testPool),
		&hStubClient{stat: &pkgstorage.ObjectInfo{Size: 21, ETag: "etag-h", ContentType: "image/png"}, getBody: []byte("avatar-proxy-bytes")},
		cfg, slog.Default())
	h := handler.NewStorageHandler(svc, &denyAuth{allow: allowAuth}, audit)

	gin.SetMode(gin.TestMode)
	r := gin.New()
	g := r.Group("/api/v1", func(c *gin.Context) {
		c.Set("userID", uid)
		// 复刻中间件 request_id 注入（response.OK 读 gin 键——utils/response:33）
		c.Set("request_id", "req-it-storage")
		c.Next()
	})
	g.POST("/storage/presign/upload", h.PresignUpload)
	g.POST("/storage/avatar/bind", h.BindAvatar)
	g.GET("/tickets/:id/attachments", h.ListAttachments)
	g.POST("/tickets/attachments/delete", h.DeleteAttachment)
	g.POST("/tickets/attachments/confirm", h.Confirm)
	g.GET("/files/*key", h.ProxyDownload)
	return r, uid
}

type envelope struct {
	Code      int             `json:"code"`
	Message   string          `json:"message"`
	Data      json.RawMessage `json:"data"`
	RequestID string          `json:"request_id"`
}

func post(t *testing.T, r *gin.Engine, path string, body any) (*httptest.ResponseRecorder, envelope) {
	t.Helper()
	raw, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var env envelope
	if w.Body.Len() > 0 {
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &env), "响应须为合法信封: %s", w.Body.String())
	}
	return w, env
}

func TestStorageHandler_Contract(t *testing.T) {
	validUpload := map[string]any{
		"purpose": "avatar", "file_name": "a.png", "mime_type": "image/png", "size_bytes": "100",
	}

	t.Run("presign 正常——信封+响应形态", func(t *testing.T) {
		r, _ := newStorageRouter(t)
		w, env := post(t, r, "/api/v1/storage/presign/upload", validUpload)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 0, env.Code)
		assert.NotEmpty(t, env.RequestID, "信封必带 request_id")
		var data struct {
			UploadURL string            `json:"upload_url"`
			Method    string            `json:"method"`
			Headers   map[string]string `json:"headers"`
			ObjectKey string            `json:"object_key"`
		}
		require.NoError(t, json.Unmarshal(env.Data, &data))
		assert.Equal(t, "PUT", data.Method)
		assert.Equal(t, "image/png", data.Headers["Content-Type"])
		assert.Contains(t, data.ObjectKey, "avatars/")
	})

	t.Run("presign MIME 黑名单——400+91002", func(t *testing.T) {
		r, _ := newStorageRouter(t)
		in := map[string]any{"purpose": "avatar", "file_name": "a.html", "mime_type": "text/html", "size_bytes": "10"}
		w, env := post(t, r, "/api/v1/storage/presign/upload", in)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, errcode.ErrFileTypeNotAllowed.Code, env.Code)
	})

	t.Run("presign size 超限——400+91001", func(t *testing.T) {
		r, _ := newStorageRouter(t)
		in := map[string]any{"purpose": "avatar", "file_name": "a.png", "mime_type": "image/png", "size_bytes": "99999999"}
		w, env := post(t, r, "/api/v1/storage/presign/upload", in)
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, errcode.ErrFileTooLarge.Code, env.Code)
	})

	t.Run("presign 缺 purpose——400 参数错误", func(t *testing.T) {
		r, _ := newStorageRouter(t)
		w, env := post(t, r, "/api/v1/storage/presign/upload", map[string]any{"file_name": "a", "mime_type": "image/png", "size_bytes": "1"})
		assert.Equal(t, http.StatusBadRequest, w.Code)
		assert.Equal(t, errcode.ErrInvalidParams.Code, env.Code)
	})

	t.Run("confirm 不存在——404+91003", func(t *testing.T) {
		r, _ := newStorageRouter(t)
		w, env := post(t, r, "/api/v1/tickets/attachments/confirm", map[string]any{"object_key": "avatars/2026/10/nope.png"})
		assert.Equal(t, http.StatusNotFound, w.Code)
		assert.Equal(t, errcode.ErrFileNotFound.Code, env.Code)
	})

	t.Run("confirm 正常→重复——200 后 409+91004", func(t *testing.T) {
		r, _ := newStorageRouter(t)
		_, env := post(t, r, "/api/v1/storage/presign/upload", validUpload)
		require.Equal(t, 0, env.Code)
		var pre struct {
			ObjectKey string `json:"object_key"`
		}
		require.NoError(t, json.Unmarshal(env.Data, &pre))

		w, env2 := post(t, r, "/api/v1/tickets/attachments/confirm", map[string]any{"object_key": pre.ObjectKey})
		require.Equal(t, http.StatusOK, w.Code)
		var f struct {
			Status string `json:"status"`
		}
		require.NoError(t, json.Unmarshal(env2.Data, &f))
		assert.Equal(t, "confirmed", f.Status)

		w3, env3 := post(t, r, "/api/v1/tickets/attachments/confirm", map[string]any{"object_key": pre.ObjectKey})
		assert.Equal(t, http.StatusConflict, w3.Code)
		assert.Equal(t, errcode.ErrFileAlreadyBound.Code, env3.Code)
	})

	t.Run("代理下载——响应头契约（A.4 #5）", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		_, env := post(t, r2, "/api/v1/storage/presign/upload", validUpload)
		require.Equal(t, 0, env.Code)
		var pre struct {
			ObjectKey string `json:"object_key"`
		}
		require.NoError(t, json.Unmarshal(env.Data, &pre))
		_, env2 := post(t, r2, "/api/v1/tickets/attachments/confirm", map[string]any{"object_key": pre.ObjectKey})
		require.Equal(t, 0, env2.Code)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+pre.ObjectKey, nil)
		req.Header.Set("Authorization", "Bearer x") // 组中间件已注入 userID——header 值仅占位
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "avatar-proxy-bytes", w.Body.String())
		assert.Equal(t, "image/png", w.Header().Get("Content-Type"))
		assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
		assert.Equal(t, "private", w.Header().Get("Cache-Control"))
		assert.Equal(t, `"etag-h"`, w.Header().Get("ETag"))
		assert.Contains(t, w.Header().Get("Content-Disposition"), "inline")
		assert.Contains(t, w.Header().Get("Content-Disposition"), "filename*=UTF-8''")
		// S5 评审 P3（2026-10-20）：实测 size 在手——补 Content-Length（进度条/长度校验）
		assert.Equal(t, "21", w.Header().Get("Content-Length"))
	})

	// S5 评审 P2-5（2026-10-20）：审计白名单正向化——仅 ticket 态 200 落行
	//（avatar 高频渲染免审计，A.4 #5 收窄口径的有测化）。
	t.Run("代理下载——审计白名单（仅 ticket 态 200 落行）", func(t *testing.T) {
		rec := &auditRecorder{}
		r2, uid := newStorageRouterWith(t, rec, true)

		// avatar 面：确认对象下载——零审计行
		_, env := post(t, r2, "/api/v1/storage/presign/upload", validUpload)
		require.Equal(t, 0, env.Code)
		var pre struct {
			ObjectKey string `json:"object_key"`
		}
		require.NoError(t, json.Unmarshal(env.Data, &pre))
		_, env2 := post(t, r2, "/api/v1/tickets/attachments/confirm", map[string]any{"object_key": pre.ObjectKey})
		require.Equal(t, 0, env2.Code)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+pre.ObjectKey, nil)
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, req)
		require.Equal(t, http.StatusOK, w.Code)
		assert.Empty(t, rec.entries, "avatar 下载免审计（A.7 收窄——热路径写放大）")

		// ticket 面：直插 confirmed 绑定对象 → 下载——恰一行审计（含 key）
		tkID := insertHTicket(t, uid)
		akey := "tickets/2026/10/audit-face.png"
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by)
			VALUES ('it-h', $1, 'ticket_attachment', 'confirmed', 'a.png', 'image/png', $2)`, akey, uid)
		require.NoError(t, err)
		var foID int64
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT id FROM file_objects WHERE bucket='it-h' AND object_key=$1`, akey).Scan(&foID))
		_, err = testPool.Exec(context.Background(),
			`INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by) VALUES ($1, $2, $3)`, tkID, foID, uid)
		require.NoError(t, err)

		req2 := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+akey, nil)
		w2 := httptest.NewRecorder()
		r2.ServeHTTP(w2, req2)
		require.Equal(t, http.StatusOK, w2.Code)
		require.Len(t, rec.entries, 1, "ticket 态 200 恰落一行")
		assert.Equal(t, http.MethodGet, rec.entries[0].Method)
		assert.Contains(t, rec.entries[0].Path, akey, "审计行含对象键（A.7 谁/何时/key/结果）")
	})

	t.Run("代理下载——If-None-Match 304（零审计行）", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		_, env := post(t, r2, "/api/v1/storage/presign/upload", validUpload)
		require.Equal(t, 0, env.Code)
		var pre struct {
			ObjectKey string `json:"object_key"`
		}
		require.NoError(t, json.Unmarshal(env.Data, &pre))
		_, err2 := post(t, r2, "/api/v1/tickets/attachments/confirm", map[string]any{"object_key": pre.ObjectKey})
		require.Equal(t, 0, err2.Code)

		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/"+pre.ObjectKey, nil)
		req.Header.Set("If-None-Match", `"etag-h"`)
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotModified, w.Code)
		assert.Empty(t, w.Body.String())
	})

	t.Run("avatar bind——200 回显+users.avatar 落键+重复幂等", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		_, env := post(t, r2, "/api/v1/storage/presign/upload", validUpload)
		require.Equal(t, 0, env.Code)
		var pre struct {
			ObjectKey string `json:"object_key"`
		}
		require.NoError(t, json.Unmarshal(env.Data, &pre))
		_, err2 := post(t, r2, "/api/v1/tickets/attachments/confirm", map[string]any{"object_key": pre.ObjectKey})
		require.Equal(t, 0, err2.Code)

		w, env3 := post(t, r2, "/api/v1/storage/avatar/bind", map[string]any{"object_key": pre.ObjectKey})
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, 0, env3.Code)
		w4, _ := post(t, r2, "/api/v1/storage/avatar/bind", map[string]any{"object_key": pre.ObjectKey})
		assert.Equal(t, http.StatusOK, w4.Code, "同键重复 bind 幂等")
	})

	t.Run("ticket 态 presign——L2 deny 403", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		w2, env2 := post(t, r2, "/api/v1/storage/presign/upload", map[string]any{
			"purpose": "ticket_attachment", "file_name": "a.png", "mime_type": "image/png",
			"size_bytes": "10", "ticket_id": "999",
		})
		assert.Equal(t, http.StatusForbidden, w2.Code)
		assert.Equal(t, errcode.ErrNoPermission.Code, env2.Code)
	})

	t.Run("ticket 态 presign——缺 ticket_id 400", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		w2, env2 := post(t, r2, "/api/v1/storage/presign/upload", map[string]any{
			"purpose": "ticket_attachment", "file_name": "a.png", "mime_type": "image/png", "size_bytes": "10",
		})
		assert.Equal(t, http.StatusBadRequest, w2.Code)
		assert.Equal(t, errcode.ErrInvalidParams.Code, env2.Code)
	})

	t.Run("avatar bind——未知键同形 404", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		w2, env2 := post(t, r2, "/api/v1/storage/avatar/bind", map[string]any{"object_key": "avatars/2026/10/other.png"})
		assert.Equal(t, http.StatusNotFound, w2.Code)
		assert.Equal(t, errcode.ErrFileNotFound.Code, env2.Code)
	})

	t.Run("附件列表——L2 deny 403", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/tickets/999/attachments", nil)
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, req)
		assert.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("删附件——L2 deny 403", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		w2, env2 := post(t, r2, "/api/v1/tickets/attachments/delete", map[string]any{"ticket_id": "1", "object_key": "tickets/x.png"})
		assert.Equal(t, http.StatusForbidden, w2.Code)
		assert.Equal(t, errcode.ErrNoPermission.Code, env2.Code)
	})

	t.Run("代理下载——未知键 404", func(t *testing.T) {
		r2, _ := newStorageRouter(t)
		req := httptest.NewRequest(http.MethodGet, "/api/v1/files/avatars/2026/10/nope.png", nil)
		w := httptest.NewRecorder()
		r2.ServeHTTP(w, req)
		assert.Equal(t, http.StatusNotFound, w.Code)
	})
}

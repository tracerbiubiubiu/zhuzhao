package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

const uploadCommandActor = int64(17)
const uploadCommandTicket = int64(9007199254740993)

func TestStorageUploadCommandPresignAuthorization(t *testing.T) {
	for _, evidencePort := range []bool{false, true} {
		name := "attachment command"
		if evidencePort {
			name = "purpose-specific upload command"
		}
		t.Run(name, func(t *testing.T) {
			legacy := &uploadCommandAuthorizer{updateErr: errcode.ErrNoPermission}
			command := &uploadCommandGuard{uploadCommandAuthorizer: legacy}
			var auth TicketAuthorizer = command
			var upload *uploadPurposeGuard
			if evidencePort {
				upload = &uploadPurposeGuard{uploadCommandGuard: command, deny: true}
				auth = upload
			}
			r, client := newUploadCommandRouter(auth)
			postUploadCommandDenied(t, r, "/presign", `{"purpose":" ticket_attachment ","ticket_id":"9007199254740993","file_name":"evidence.png","mime_type":"image/png","size_bytes":"12"}`)

			require.Zero(t, legacy.updateCalls, "assembled command owns upload authorization")
			require.Empty(t, client.calls, "denied upload must not issue an object URL")
			if !evidencePort {
				require.Equal(t, []string{"presign"}, command.actions)
				return
			}
			require.Empty(t, command.actions)
			require.Equal(t, []string{"presign"}, upload.actions)
			require.Equal(t, uploadCommandActor, upload.actor)
			require.Equal(t, uploadCommandTicket, upload.ticket)
			require.NotNil(t, upload.file)
			require.Equal(t, "ticket_attachment", upload.file.Purpose)
			require.Equal(t, model.FileStatusPending, upload.file.Status)
			require.Equal(t, "evidence.png", upload.file.FileName)
			require.Equal(t, uploadCommandActor, upload.file.CreatedBy)
			require.True(t, strings.HasPrefix(upload.file.ObjectKey, "tickets/"))
			require.True(t, upload.hasWrite, "service passes its pending-write callback to the guard")
		})
	}
}

func TestStorageUploadCommandLegacyAuthorization(t *testing.T) {
	for _, tc := range []struct {
		name, route, body string
	}{
		{"attachment presign", "/presign", `{"purpose":"ticket_attachment","ticket_id":"9007199254740993","file_name":"evidence.png","mime_type":"image/png","size_bytes":"12"}`},
		{"richtext presign", "/presign", `{"purpose":"ticket_richtext","ticket_id":"9007199254740993","file_name":"inline.png","mime_type":"image/png","size_bytes":"12"}`},
		{"confirm", "/confirm", `{"ticket_id":"9007199254740993","object_key":"tickets/2026/11/evidence.png"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			auth := &uploadCommandAuthorizer{updateErr: errcode.ErrNoPermission}
			r, client := newUploadCommandRouter(auth)
			postUploadCommandDenied(t, r, tc.route, tc.body)
			require.Equal(t, 1, auth.updateCalls)
			require.Equal(t, uploadCommandActor, auth.actor)
			require.Equal(t, uploadCommandTicket, auth.ticket)
			require.Empty(t, client.calls)
		})
	}
}

func TestStorageUploadCommandKeepsRichtextAndDeleteUpdateGuards(t *testing.T) {
	for _, tc := range []struct {
		name, route, body, action string
		legacyDenies              bool
		wantLegacyCalls           int
	}{
		{"richtext presign", "/presign", `{"purpose":"ticket_richtext","ticket_id":"9007199254740993","file_name":"inline.png","mime_type":"image/png","size_bytes":"12"}`, "presign", true, 0},
		{"delete legacy authorization", "/delete", `{"ticket_id":"9007199254740993","object_key":"tickets/2026/11/evidence.png"}`, "", true, 1},
		{"delete persistent command", "/delete", `{"ticket_id":"9007199254740993","object_key":"tickets/2026/11/evidence.png"}`, "delete", false, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			legacy := &uploadCommandAuthorizer{}
			if tc.legacyDenies {
				legacy.updateErr = errcode.ErrNoPermission
			}
			command := &uploadCommandGuard{uploadCommandAuthorizer: legacy}
			// This port would allow an upload. Richtext and delete must never use it.
			upload := &uploadPurposeGuard{uploadCommandGuard: command}
			r, client := newUploadCommandRouter(upload)
			postUploadCommandDenied(t, r, tc.route, tc.body)
			require.Equal(t, tc.wantLegacyCalls, legacy.updateCalls)
			if tc.action == "" {
				require.Empty(t, command.actions)
			} else {
				require.Equal(t, []string{tc.action}, command.actions)
			}
			require.Empty(t, upload.actions, "evidence-upload capability must not authorize other writes")
			require.Empty(t, client.calls)
		})
	}
}

type uploadCommandAuthorizer struct {
	updateErr   error
	updateCalls int
	actor       int64
	ticket      int64
}

func (a *uploadCommandAuthorizer) AuthorizeTicketAttachment(_ context.Context, actor, ticket int64) error {
	a.updateCalls++
	a.actor, a.ticket = actor, ticket
	return a.updateErr
}

func (*uploadCommandAuthorizer) AuthorizeTicketFileRead(context.Context, int64, int64) error {
	return errcode.ErrNoPermission
}

type uploadCommandGuard struct {
	*uploadCommandAuthorizer
	actions []string
}

func (g *uploadCommandGuard) AttachmentCommand(_ context.Context, action string, _, _ int64, _ func(context.Context) error) error {
	g.actions = append(g.actions, action)
	return errcode.ErrNoPermission
}

type uploadPurposeGuard struct {
	*uploadCommandGuard
	deny     bool
	actions  []string
	actor    int64
	ticket   int64
	file     *model.FileObject
	hasWrite bool
}

func (g *uploadPurposeGuard) AttachmentUploadCommand(_ context.Context, action string, actor, ticket int64, file *model.FileObject, write func(context.Context) error) error {
	g.actions = append(g.actions, action)
	g.actor, g.ticket, g.file, g.hasWrite = actor, ticket, file, write != nil
	if g.deny {
		return errcode.ErrNoPermission
	}
	return nil
}

func newUploadCommandRouter(auth TicketAuthorizer) (*gin.Engine, *uploadCommandClient) {
	cfg := &pkgstorage.StorageConfig{Enabled: true, Bucket: "unit-upload-command", PresignUploadTTL: "15m"}
	cfg.ApplyDefaults()
	client := &uploadCommandClient{}
	// A rejecting guard must return before either persistence or external I/O.
	svc := storageservice.NewService(nil, client, pkgstorage.NewConfigStore(cfg), slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := NewStorageHandler(svc, auth, nil)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", uploadCommandActor)
		c.Set("request_id", "req-unit-upload-command")
		c.Next()
	})
	r.POST("/presign", h.PresignUpload)
	r.POST("/confirm", h.Confirm)
	r.POST("/delete", h.DeleteAttachment)
	return r, client
}

func postUploadCommandDenied(t *testing.T, r *gin.Engine, route, body string) {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, route, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	var result struct {
		Code      int    `json:"code"`
		RequestID string `json:"request_id"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Equal(t, errcode.ErrNoPermission.Code, result.Code)
	require.Equal(t, "req-unit-upload-command", result.RequestID)
}

type uploadCommandClient struct{ calls []string }

func (c *uploadCommandClient) unexpected(action string) error {
	c.calls = append(c.calls, action)
	return errors.New("unexpected object-storage call: " + action)
}

func (c *uploadCommandClient) PresignPut(context.Context, string, string, time.Duration) (*pkgstorage.PresignedPut, error) {
	return nil, c.unexpected("presign put")
}

func (c *uploadCommandClient) PresignGet(context.Context, string, time.Duration) (string, error) {
	return "", c.unexpected("presign get")
}

func (c *uploadCommandClient) StatObject(context.Context, string) (*pkgstorage.ObjectInfo, error) {
	return nil, c.unexpected("stat")
}

func (c *uploadCommandClient) GetObject(context.Context, string, string) (io.ReadCloser, *pkgstorage.ObjectInfo, error) {
	return nil, nil, c.unexpected("get")
}

func (c *uploadCommandClient) RemoveObject(context.Context, string) error {
	return c.unexpected("remove")
}

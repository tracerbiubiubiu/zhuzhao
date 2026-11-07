//go:build integration

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
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// Exercise real HTTP serialization after real repository reads. S3 and the L2
// decision are stubs; this test does not claim to validate either boundary.
func TestStorageHandler_IDWireContract(t *testing.T) {
	ctx := context.Background()
	name := fmt.Sprintf("E884%d", time.Now().UnixNano())
	bucket := "storage-id-" + name
	var uid int64
	require.NoError(t, testPool.QueryRow(ctx,
		`INSERT INTO users(employee_no,username,password,status) VALUES($1,$1,'synthetic',1) RETURNING id`, name).Scan(&uid))
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM ticket_attachments WHERE file_object_id IN (SELECT id FROM file_objects WHERE bucket=$1)`, bucket)
		_, _ = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket=$1`, bucket)
		_, _ = testPool.Exec(ctx, `DELETE FROM users WHERE id=$1`, uid)
	})
	ticketID := insertHTicket(t, uid)
	cfg := &pkgstorage.StorageConfig{Enabled: true, Bucket: bucket}
	cfg.ApplyDefaults()
	content := []byte("synthetic-file")
	client := &hStubClient{stat: &pkgstorage.ObjectInfo{Size: int64(len(content)), ETag: "etag-id", ContentType: "image/png"}, getBody: content}
	h := handler.NewStorageHandler(storageservice.NewService(repository.NewStorageRepo(testPool), client,
		pkgstorage.NewConfigStore(cfg), slog.Default()), &denyAuth{allow: true}, nil)
	gin.SetMode(gin.TestMode)
	router := gin.New()
	group := router.Group("/api/v1", func(c *gin.Context) { c.Set("userID", uid) })
	group.POST("/tickets/attachments/confirm", h.Confirm)
	group.POST("/storage/avatar/bind", h.BindAvatar)
	group.GET("/tickets/:id/attachments", h.ListAttachments)
	group.POST("/tickets/attachments/delete", h.DeleteAttachment)
	group.GET("/files/*key", h.ProxyDownload)
	server := httptest.NewServer(router)
	t.Cleanup(server.Close)
	httpClient := &http.Client{Timeout: 5 * time.Second}

	request := func(method, path string, input any) json.RawMessage {
		t.Helper()
		body, err := json.Marshal(input)
		require.NoError(t, err)
		req, err := http.NewRequest(method, server.URL+path, bytes.NewReader(body))
		require.NoError(t, err)
		req.Header.Set("Content-Type", "application/json")
		resp, err := httpClient.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		require.Equal(t, http.StatusOK, resp.StatusCode, "%s: %s", path, raw)
		t.Logf("STORAGE_HTTP %s %s", path, raw)
		var env envelope
		require.NoError(t, json.Unmarshal(raw, &env))
		require.Zero(t, env.Code)
		return env.Data
	}

	ids := []int64{1 << 53, 1<<53 + 1}
	for index, id := range ids {
		key := fmt.Sprintf("avatars/id-contract-%d.png", index)
		_, err := testPool.Exec(ctx, `INSERT INTO file_objects(id,bucket,object_key,purpose,status,file_name,mime_type,created_by)
			VALUES($1,$2,$3,'avatar','pending','synthetic.png','image/png',$4)`, id, bucket, key, uid)
		require.NoError(t, err)
		for _, path := range []string{"/api/v1/tickets/attachments/confirm", "/api/v1/storage/avatar/bind"} {
			var row map[string]any
			require.NoError(t, json.Unmarshal(request(http.MethodPost, path, map[string]string{"object_key": key}), &row))
			assert.Equal(t, strconv.FormatInt(id, 10), row["id"], "%s must preserve an exact string ID", path)
			assert.Equal(t, strconv.FormatInt(uid, 10), row["created_by"])
			assert.Equal(t, strconv.Itoa(len(content)), row["size_bytes"], "existing size representation must remain unchanged")
			assert.Equal(t, key, row["object_key"])
		}
		var avatar string
		require.NoError(t, testPool.QueryRow(ctx, `SELECT avatar FROM users WHERE id=$1`, uid).Scan(&avatar))
		assert.Equal(t, key, avatar, "avatar binding still stores the object key")
	}

	for index, id := range ids {
		key := fmt.Sprintf("tickets/id-contract-%d.png", index)
		_, err := testPool.Exec(ctx, `INSERT INTO file_objects(id,bucket,object_key,purpose,status,file_name,mime_type,size_bytes,etag,created_by)
			VALUES($1,$2,$3,'ticket_attachment','confirmed','synthetic.png','image/png',$4,'etag-id',$5)`, id+2, bucket, key, len(content), uid)
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, `INSERT INTO ticket_attachments(id,ticket_id,file_object_id,uploaded_by) VALUES($1,$2,$3,$4)`, id, ticketID, id+2, uid)
		require.NoError(t, err)
	}
	path := fmt.Sprintf("/api/v1/tickets/%d/attachments", ticketID)
	var rows []map[string]any
	require.NoError(t, json.Unmarshal(request(http.MethodGet, path, nil), &rows))
	require.Len(t, rows, 2)
	for index, row := range rows {
		assert.Equal(t, strconv.FormatInt(ids[index], 10), row["id"])
		assert.Equal(t, strconv.FormatInt(ticketID, 10), row["ticket_id"])
		assert.Equal(t, strconv.FormatInt(uid, 10), row["uploaded_by"])
		assert.Equal(t, strconv.Itoa(len(content)), row["size_bytes"])
		key := fmt.Sprintf("tickets/id-contract-%d.png", index)
		assert.Equal(t, key, row["object_key"])
		resp, err := httpClient.Get(server.URL + "/api/v1/files/" + key)
		require.NoError(t, err)
		raw, err := io.ReadAll(resp.Body)
		require.NoError(t, resp.Body.Close())
		require.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		assert.Equal(t, content, raw, "proxy download still addresses the exact object key")
		request(http.MethodPost, "/api/v1/tickets/attachments/delete", map[string]string{
			"ticket_id": strconv.FormatInt(ticketID, 10), "object_key": key,
		})
	}
	var remaining []any
	require.NoError(t, json.Unmarshal(request(http.MethodGet, path, nil), &remaining))
	assert.Empty(t, remaining, "delete still addresses each object key independently")
}

//go:build integration

// B5-3 批3：附件列表/删除/下载扩面——测试先行（先红后绿）。
// 场景：ticket 态下载（绑定单 L2 回调+放行）/ 未绑定 ticket 态对象下载拒 /
// 附件列表（JOIN 视图）/ 删除（L2 面 handler 判定——service 层 Unlink+弃旧）/
// avatar 下载回归（无 L2 回调）。
package storage_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

func TestAttachmentAPI_Scenarios(t *testing.T) {
	uid := insertUser(t)

	newSvc := func(t *testing.T) (*storageservice.Service, *stubClient) {
		t.Helper()
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		})
		cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m"}
		cfg.ApplyDefaults()
		stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}, getBody: []byte("png")}
		return storageservice.NewService(repository.NewStorageRepo(testPool), stub, cfg, testLogger()), stub
	}

	// bound 建 ticket 态已绑定附件（走全链 presign→confirm(ticket_id)）。
	bound := func(t *testing.T, svc *storageservice.Service) (key string, ticketID int64) {
		t.Helper()
		tkID := insertTicket(t, uid)
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "ticket_attachment", FileName: "附.png", MIMEType: "image/png", SizeBytes: 10, TicketID: &tkID,
		}, uid)
		require.NoError(t, err)
		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey, TicketID: &tkID}, uid)
		require.NoError(t, err)
		return resp.ObjectKey, tkID
	}

	t.Run("ticket 态下载——绑定单 L2 回调放行", func(t *testing.T) {
		svc, _ := newSvc(t)
		key, tkID := bound(t, svc)
		var seenTicket int64
		dl, err := svc.Download(context.Background(), key, "", func(ticketID int64) error {
			seenTicket = ticketID
			return nil
		})
		require.NoError(t, err)
		defer dl.Body.Close()
		assert.Equal(t, tkID, seenTicket, "L2 回调收到绑定单 ID")
		assert.Equal(t, "ticket_attachment", dl.File.Purpose)
	})

	t.Run("ticket 态下载——L2 拒绝透传", func(t *testing.T) {
		svc, _ := newSvc(t)
		key, _ := bound(t, svc)
		_, err := svc.Download(context.Background(), key, "", func(int64) error {
			return errcode.ErrNoPermission
		})
		assert.Equal(t, errcode.ErrNoPermission.Code, codeOf(t, err))
	})

	t.Run("未绑定 ticket 态对象下载拒（悬挂态防御）", func(t *testing.T) {
		svc, _ := newSvc(t)
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "ticket_attachment", FileName: "h.png", MIMEType: "image/png", SizeBytes: 10,
			TicketID: func() *int64 { id := insertTicket(t, uid); return &id }(),
		}, uid)
		require.NoError(t, err)
		// 只 confirm 不落绑（构造未绑定防御态：直改行绕过 ConfirmAndBind）
		_, _ = testPool.Exec(context.Background(),
			`UPDATE file_objects SET status='confirmed' WHERE bucket=$1 AND object_key=$2`, itBucket, resp.ObjectKey)
		_, err = svc.Download(context.Background(), resp.ObjectKey, "", func(int64) error { return nil })
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err), "ticket 态未绑定对象不可下载")
	})

	t.Run("avatar 下载回归——无 L2 回调路径", func(t *testing.T) {
		svc, _ := newSvc(t)
		key := presigned(t, svc, uid, "image/png")
		_, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key}, uid)
		require.NoError(t, err)
		_, err = svc.Download(context.Background(), key, "", nil)
		assert.NoError(t, err, "avatar 面无需 L2 回调（nil 放行）")
	})

	t.Run("附件列表——JOIN 视图", func(t *testing.T) {
		svc, _ := newSvc(t)
		key, tkID := bound(t, svc)
		list, err := svc.ListTicketAttachments(context.Background(), tkID)
		require.NoError(t, err)
		require.Len(t, list, 1)
		assert.Equal(t, key, list[0].ObjectKey)
		assert.Equal(t, "附.png", list[0].FileName)
		assert.Equal(t, "image/png", list[0].MIMEType)
	})

	t.Run("删除附件——Unlink+对象弃旧", func(t *testing.T) {
		svc, stub := newSvc(t)
		key, tkID := bound(t, svc)
		err := svc.DeleteTicketAttachment(context.Background(), tkID, key, uid)
		require.NoError(t, err)
		assert.Equal(t, []string{key}, stub.removedKeys(), "对象本体 best-effort 删除")
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tkID).Scan(&n))
		assert.Equal(t, 0, n, "关联行已删")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, key).Scan(&status))
		assert.Equal(t, model.FileStatusDeleted, status)
	})

	t.Run("删除他人附件——归属校验拒", func(t *testing.T) {
		svc, _ := newSvc(t)
		_, tkID := bound(t, svc)
		err := svc.DeleteTicketAttachment(context.Background(), tkID, "tickets/2026/10/nope.png", uid)
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})
}

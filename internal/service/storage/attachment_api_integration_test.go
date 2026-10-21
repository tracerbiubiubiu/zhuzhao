//go:build integration

// B5-3 批3：附件列表/删除/下载扩面——测试先行（先红后绿）。
// 场景：ticket 态下载（绑定单 L2 回调+放行）/ 未绑定 ticket 态对象下载拒 /
// 附件列表（JOIN 视图）/ 删除（L2 面 handler 判定——service 层 Unlink+弃旧）/
// avatar 下载回归（无 L2 回调）。
package storage_test

import (
	"context"
	"fmt"
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
		err := svc.DeleteTicketAttachment(context.Background(), tkID, key)
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
		err := svc.DeleteTicketAttachment(context.Background(), tkID, "tickets/2026/10/nope.png")
		assert.Equal(t, errcode.ErrFileNotFound.Code, codeOf(t, err))
	})

	// S5 评审 P2-1（2026-10-20）删除口径批：对象物理删失败不阻塞——行先翻
	// deleted（行态优先），对象残留由 GC 二扫收（修复前 deleted 无扫描覆盖=泄漏）。
	t.Run("删附件对象删除失败——仍标 deleted（GC 二扫收残留）", func(t *testing.T) {
		svc, stub := newSvc(t)
		stub.removeErr = assert.AnError
		key, tkID := bound(t, svc)
		err := svc.DeleteTicketAttachment(context.Background(), tkID, key)
		require.NoError(t, err, "best-effort 物理删失败不阻塞删关联+落标")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, key).Scan(&status))
		assert.Equal(t, model.FileStatusDeleted, status, "行已翻 deleted——对象残留 GC 二扫收")
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tkID).Scan(&n))
		assert.Zero(t, n, "关联行已删")
	})

	// S5 评审 P2-2/D1-3（2026-10-20）健壮性批：①GetBoundTicketID 跨桶隔离
	//（修复前裸查 object_key 无 bucket 条件）；②非 ErrNoRows 错误上抛
	//（修复前 DB 故障吞成 404 哨兵、绕过 5xx 告警面）。
	// 拍板 P2（2026-10-21）：richtext 内嵌图不计入附件额度——05 §3 两档口径
	//（"单工单附件 ≤20；richtext 内嵌图软限"）。贴满 20 图后传普通附件应成功。
	t.Run("贴满 20 张内嵌图仍可传普通附件", func(t *testing.T) {
		svc, _ := newSvc(t)
		tkID := insertTicket(t, uid)
		ctx := context.Background()
		for i := 0; i < 20; i++ {
			key := fmt.Sprintf("tickets/2026/10/rt-%d.png", i)
			var foID int64
			require.NoError(t, testPool.QueryRow(ctx, `
				INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by)
				VALUES ($1, $2, 'ticket_richtext', 'confirmed', 'r.png', 'image/png', $3) RETURNING id`,
				itBucket, key, uid).Scan(&foID))
			_, err := testPool.Exec(ctx,
				`INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by) VALUES ($1, $2, $3)`, tkID, foID, uid)
			require.NoError(t, err)
		}
		resp, err := svc.PresignUpload(ctx, &model.PresignUploadInput{
			Purpose: "ticket_attachment", FileName: "a.png", MIMEType: "image/png", SizeBytes: 10, TicketID: &tkID,
		}, uid)
		require.NoError(t, err)
		_, err = svc.Confirm(ctx, &model.ConfirmFileInput{ObjectKey: resp.ObjectKey, TicketID: &tkID}, uid)
		require.NoError(t, err, "richtext 图不计入附件额度——贴满 20 图仍可传普通附件")
	})

	// 拍板 P3（2026-10-21）：richtext 内嵌图不单独删——A.7「随工单」口径。
	// 删内嵌图=MD 正文死链且无恢复路径，API 面拒 400。
	t.Run("删内嵌图拒 400（随工单生命周期）", func(t *testing.T) {
		svc, _ := newSvc(t)
		tkID := insertTicket(t, uid)
		key := "tickets/2026/10/embedded.png"
		var foID int64
		require.NoError(t, testPool.QueryRow(context.Background(), `
			INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by)
			VALUES ($1, $2, 'ticket_richtext', 'confirmed', 'e.png', 'image/png', $3) RETURNING id`,
			itBucket, key, uid).Scan(&foID))
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by) VALUES ($1, $2, $3)`, tkID, foID, uid)
		require.NoError(t, err)

		err = svc.DeleteTicketAttachment(context.Background(), tkID, key)
		require.Error(t, err, "内嵌图不可经删附件端点单独删（修复前 200）")
		e, ok := err.(*errcode.Error)
		require.True(t, ok)
		assert.Equal(t, errcode.ErrInvalidParams.Code, e.Code)
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tkID).Scan(&n))
		assert.Equal(t, 1, n, "关联行不得被删")
	})

	t.Run("GetBoundTicketID——跨桶隔离+DB 错误上抛", func(t *testing.T) {
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = 'other-h'`)
		})
		tkID := insertTicket(t, uid)
		var foID int64
		require.NoError(t, testPool.QueryRow(context.Background(), `
			INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by)
			VALUES ('other-h', 'tickets/2026/10/cross-bucket.png', 'ticket_attachment', 'confirmed', 'x.png', 'image/png', $1)
			RETURNING id`, uid).Scan(&foID))
		_, err := testPool.Exec(context.Background(),
			`INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by) VALUES ($1, $2, $3)`, tkID, foID, uid)
		require.NoError(t, err)

		repo := repository.NewStorageRepo(testPool)
		_, err = repo.GetBoundTicketID(context.Background(), itBucket, "tickets/2026/10/cross-bucket.png")
		assert.ErrorIs(t, err, repository.ErrStorageObjectNotFound,
			"itBucket 查询不得命中 other-h 桶对象（修复前裸查命中）")

		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, err = repo.GetBoundTicketID(ctx, itBucket, "tickets/2026/10/any.png")
		assert.ErrorIs(t, err, context.Canceled, "DB 错误须上抛（修复前吞成 404 哨兵）")
	})
}

//go:build integration

// B5-3 批2：ticket 态附件链——测试先行（先红后绿）。
// 场景：presign ticket_attachment 落 pending / confirm 带 ticket_id 落绑
// （关联行+uploaded_by）/ 重复绑同单 91004 / ticket 态 confirm 缺 ticket_id 拒 /
// 建单前态 confirm 悬挂不落绑（回归）。
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

// insertTicket 直插最小工单行（FK 载体——不 经 ticket service，本测试焦点在 storage 链）。
// 自足种子：root org + incident 类型（storage 包测试库无 ticket 包的 seedMinimal）。
func insertTicket(t *testing.T, uid int64) int64 {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO organizations (code, name, path, is_virtual, status, sort_order, is_system)
		SELECT 'root', 'Root', 'root', false, 1, 0, true
		WHERE NOT EXISTS (SELECT 1 FROM organizations WHERE code = 'root')`)
	require.NoError(t, err)
	_, err = testPool.Exec(context.Background(), `
		INSERT INTO ticket_types (code, name, states, transitions)
		SELECT 'incident', '事件', '["open","assigned","closed"]', '{}'
		WHERE NOT EXISTS (SELECT 1 FROM ticket_types WHERE code = 'incident')`)
	require.NoError(t, err)
	var orgID int64
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT id FROM organizations WHERE code = 'root' LIMIT 1`).Scan(&orgID))
	var id int64
	require.NoError(t, testPool.QueryRow(context.Background(), `
		INSERT INTO tickets (type_code, title, description, priority, status, created_by, org_id, org_path)
		VALUES ('incident', 'B5-3 附件链测试', 'x', 3, 'open', $1, $2,
			(SELECT path FROM organizations WHERE id = $2))
		RETURNING id`, uid, orgID).Scan(&id))
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM tickets WHERE id = $1`, id)
	})
	return id
}

func TestTicketAttachment_Scenarios(t *testing.T) {
	uid := insertUser(t)

	newSvc := func(t *testing.T) *storageservice.Service {
		t.Helper()
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		})
		cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m"}
		cfg.ApplyDefaults()
		return storageservice.NewService(repository.NewStorageRepo(testPool),
			&stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}, getBody: []byte("png")},
			cfg, testLogger())
	}

	t.Run("presign ticket_attachment 落 pending", func(t *testing.T) {
		svc := newSvc(t)
		tkID := insertTicket(t, uid)
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "ticket_attachment", FileName: "附.png", MIMEType: "image/png", SizeBytes: 10,
			TicketID: &tkID,
		}, uid)
		require.NoError(t, err)
		assert.Contains(t, resp.ObjectKey, "tickets/")
		repo := repository.NewStorageRepo(testPool)
		f, err := repo.GetByObjectKey(context.Background(), itBucket, resp.ObjectKey)
		require.NoError(t, err)
		assert.Equal(t, "ticket_attachment", f.Purpose)
		assert.Equal(t, model.FileStatusPending, f.Status)
	})

	t.Run("confirm 带 ticket_id 落绑关联行", func(t *testing.T) {
		svc := newSvc(t)
		tkID := insertTicket(t, uid)
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "ticket_attachment", FileName: "附.png", MIMEType: "image/png", SizeBytes: 10,
			TicketID: &tkID,
		}, uid)
		require.NoError(t, err)

		f, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{
			ObjectKey: resp.ObjectKey, TicketID: &tkID,
		}, uid)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusConfirmed, f.Status)

		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1 AND uploaded_by=$2`, tkID, uid).Scan(&n))
		assert.Equal(t, 1, n, "关联行落库")
	})

	t.Run("重复绑同单 91004", func(t *testing.T) {
		svc := newSvc(t)
		tkID := insertTicket(t, uid)
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "ticket_attachment", FileName: "a.png", MIMEType: "image/png", SizeBytes: 10,
			TicketID: &tkID,
		}, uid)
		require.NoError(t, err)
		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey, TicketID: &tkID}, uid)
		require.NoError(t, err)
		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey, TicketID: &tkID}, uid)
		assert.Equal(t, errcode.ErrFileAlreadyBound.Code, codeOf(t, err))
	})

	t.Run("ticket 态 confirm 缺 ticket_id 拒绝", func(t *testing.T) {
		svc := newSvc(t)
		tkID2 := insertTicket(t, uid)
		_ = tkID2
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "ticket_attachment", FileName: "a.png", MIMEType: "image/png", SizeBytes: 10,
			TicketID: &tkID2,
		}, uid)
		require.NoError(t, err)
		_, err = svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey}, uid)
		assert.Equal(t, errcode.ErrInvalidParams.Code, codeOf(t, err))
	})

	t.Run("建单前态 confirm 悬挂不落绑（回归）", func(t *testing.T) {
		svc := newSvc(t)
		tkID := insertTicket(t, uid)
		resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "attachment_create", FileName: "建单前.png", MIMEType: "image/png", SizeBytes: 10,
		}, uid)
		require.NoError(t, err)
		f, err := svc.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey}, uid)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusConfirmed, f.Status)
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, tkID).Scan(&n))
		assert.Equal(t, 0, n, "建单前态 confirm 不落 ticket_attachments（A.5 建单前链）")
	})
}

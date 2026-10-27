//go:build integration

// B5-1 批6b-1：GC 孤儿对象扫描——测试先行（先红后绿，场景先于实现）。
// 场景底稿 = 05 号 A.3 修正②：GC 扫 status=pending AND created_at < now()-orphan_gc_days。
package storage_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// removeRecorder 记录 RemoveObject 调用的桩。
type removeRecorder struct {
	stubClient
	removed   []string
	removeErr error
}

func (r *removeRecorder) RemoveObject(_ context.Context, key string) error {
	if r.removeErr != nil {
		return r.removeErr
	}
	r.removed = append(r.removed, key)
	return nil
}

// insertPendingDirect 直插 pending 行并回拨 created_at（绕过 PresignUpload 造过期态）。
func insertPendingDirect(t *testing.T, uid int64, key string, age time.Duration, status string) {
	t.Helper()
	_, err := testPool.Exec(context.Background(), `
		INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by, created_at)
		VALUES ($1, $2, 'avatar', $3, 'f.png', 'image/png', $4, NOW() - $5::interval)`,
		itBucket, key, status, uid, age.String())
	require.NoError(t, err)
}

func TestGCJob_Scenarios(t *testing.T) {
	uid := insertUser(t)

	newGC := func(t *testing.T) (*storageservice.GCJob, *removeRecorder) {
		t.Helper()
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		})
		cfg := &pkgstorage.StorageConfig{
			Enabled: true, Endpoint: "http://stub", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m",
		}
		cfg.ApplyDefaults() // OrphanGCDays=7
		rec := &removeRecorder{}
		repo := repository.NewStorageRepo(testPool)
		return storageservice.NewGCJob(repo, rec, pkgstorage.NewConfigStore(cfg), testLogger(), false), rec
	}

	// S5 评审删除口径批（2026-10-20）：pending 孤儿路径改物理清行——A.9「对象与
	// 行均消失」终态（原 MarkDeleted 留行为中间态，改两段收敛后 pending 路径直达终态）
	t.Run("过期 pending——对象删除+行物理清除", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/expired-1.png", 8*24*time.Hour, "pending")
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Equal(t, []string{"avatars/2026/10/expired-1.png"}, rec.removed)
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/expired-1.png").Scan(&n))
		assert.Zero(t, n, "行物理清除——A.9「对象与行均消失」")
	})

	t.Run("未过期 pending 不动", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/fresh-1.png", 1*24*time.Hour, "pending")
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Empty(t, rec.removed)
	})

	// 2026-10-27 观测批注：第四段（avatar 弃置收割）存在但 newGC 默认 observe=true
	// 只记不删——本用例语义从「GC 只扫 pending」变为「observe 默认不删」，断言不变
	t.Run("过期 confirmed 不动（observe 默认挡住第四段——见 G-3 三态用例）", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/old-confirmed.png", 30*24*time.Hour, "confirmed")
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Empty(t, rec.removed)
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/old-confirmed.png").Scan(&status))
		assert.Equal(t, model.FileStatusConfirmed, status)
	})

	t.Run("重跑幂等——已 deleted 行不再处理", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/idem.png", 8*24*time.Hour, "pending")
		require.NoError(t, gc.Handle(context.Background(), nil))
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Len(t, rec.removed, 1, "二次运行零重复删除")
	})

	// F-A1（2026-10-21）：一扫改「先条件清行、后删对象」——两种失败模式都收敛：
	// ① 行清成功+对象删失败 → 补插 deleted 墓碑走二扫（非 pending 残留）；
	// ② List 与清行之间撞上并发 confirm → 条件删 0 行让位，不动物理删。
	t.Run("pending 对象删除失败——补 deleted 墓碑走二扫（F-A1）", func(t *testing.T) {
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		})
		cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket, AccessKey: "a", SecretKey: "k", PresignUploadTTL: "15m"}
		cfg.ApplyDefaults()
		rec := &removeRecorder{removeErr: assert.AnError}
		gc := storageservice.NewGCJob(repository.NewStorageRepo(testPool), rec, pkgstorage.NewConfigStore(cfg), testLogger(), false)
		insertPendingDirect(t, uid, "avatars/2026/10/failrm-tomb.png", 8*24*time.Hour, "pending")
		require.NoError(t, gc.Handle(context.Background(), nil), "单条失败不炸整批（扫描类收敛语义）")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/failrm-tomb.png").Scan(&status))
		assert.Equal(t, model.FileStatusDeleted, status, "行已清+对象删失败=补 deleted 墓碑交二扫（修复前语义=pending 残留）")
		// 二扫重试（remove 恢复成功）→ 对象删+行物理清除
		rec.removeErr = nil
		require.NoError(t, gc.Handle(context.Background(), nil))
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/failrm-tomb.png").Scan(&n))
		assert.Zero(t, n, "墓碑经二扫收敛——对象与行均消失")
		assert.Equal(t, []string{"avatars/2026/10/failrm-tomb.png"}, rec.removed, "二扫重删一次成功")
	})

	t.Run("并发 confirm 让位——刚确认对象不被 GC 删（F-A1 核心断言）", func(t *testing.T) {
		gc, rec := newGC(t)
		key := "avatars/2026/10/concurrent-confirm.png"
		insertPendingDirect(t, uid, key, 8*24*time.Hour, "pending")
		// 模拟竞态：GC 列出候选后、清行前，用户 confirm 翻态（直接 SQL 翻 confirmed）
		_, err := testPool.Exec(context.Background(),
			`UPDATE file_objects SET status='confirmed', confirmed_at=NOW() WHERE bucket=$1 AND object_key=$2`,
			itBucket, key)
		require.NoError(t, err)
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Empty(t, rec.removed, "并发 confirm 让位——对象不得被物理删（修复前=行 confirmed 在库、对象被删的数据丢失窗口）")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, key).Scan(&status))
		assert.Equal(t, model.FileStatusConfirmed, status, "行保持 confirmed——数据不丢")
	})

	t.Run("deleted 残留对象——二扫重删+行物理清除", func(t *testing.T) {
		gc, rec := newGC(t)
		insertPendingDirect(t, uid, "avatars/2026/10/dead-leftover.png", 0, "deleted")
		require.NoError(t, gc.Handle(context.Background(), nil))
		assert.Equal(t, []string{"avatars/2026/10/dead-leftover.png"}, rec.removed)
		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/dead-leftover.png").Scan(&n))
		assert.Zero(t, n, "重删成功后行物理清除")
	})

	t.Run("deleted 对象删除失败——行保持 deleted 下轮重试", func(t *testing.T) {
		gc := func() *storageservice.GCJob {
			t.Cleanup(func() {
				_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
			})
			cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket, AccessKey: "a", SecretKey: "k", PresignUploadTTL: "15m"}
			cfg.ApplyDefaults()
			rec := &removeRecorder{removeErr: assert.AnError}
			return storageservice.NewGCJob(repository.NewStorageRepo(testPool), rec, pkgstorage.NewConfigStore(cfg), testLogger(), false)
		}()
		insertPendingDirect(t, uid, "avatars/2026/10/dead-failrm.png", 0, "deleted")
		require.NoError(t, gc.Handle(context.Background(), nil), "二扫单条失败不炸整批")
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, "avatars/2026/10/dead-failrm.png").Scan(&status))
		assert.Equal(t, model.FileStatusDeleted, status, "重删失败行保持 deleted——下轮再试")
	})
}

// TestGCJob_ConfirmedOrphans F-4-4 第三段守卫（外审 F-4 遗留补课）：
// confirmed + 工单域 purpose + 无 ticket_attachments 关联 + 超 double 阈值 → 软删+回收。
func TestGCJob_ConfirmedOrphans(t *testing.T) {
	uid := insertUser(t)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
	})
	cfg := &pkgstorage.StorageConfig{
		Enabled: true, Endpoint: "http://stub", Bucket: itBucket,
		AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m",
	}
	cfg.ApplyDefaults()
	rec := &removeRecorder{}
	repo := repository.NewStorageRepo(testPool)
	// observe=true=发布默认姿态——本函数既有 avatar 豁免用例在第四段存在下仍须绿
	// （enforce 语义由下方 G-3 三态用例自带实例覆盖）
	gc := storageservice.NewGCJob(repo, rec, pkgstorage.NewConfigStore(cfg), testLogger(), true)

	seedConfirmed := func(key, purpose string, age time.Duration) {
		_, err := testPool.Exec(context.Background(), `
			INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by, confirmed_at)
			VALUES ($1, $2, $3, 'confirmed', 'f.png', 'image/png', $4, NOW() - $5::interval)`,
			itBucket, key, purpose, uid, age.String())
		require.NoError(t, err)
	}

	t.Run("悬挂建单对象超期→回收", func(t *testing.T) {
		key := "tickets/2026/10/orphan_hang.png"
		seedConfirmed(key, "attachment_create", 24*30*time.Hour) // 超 7×2 阈值
		require.NoError(t, gc.Handle(context.Background(), nil))
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE object_key=$1`, key).Scan(&status))
		assert.Equal(t, "deleted", status, "悬挂 confirmed 超期应被第三段软删")
	})

	t.Run("有关联行的 confirmed 不动（防误删在用附件）", func(t *testing.T) {
		// 测试容器自建最小种子（ organizations root 可能不存在——upsert 保证）
		var orgID int64
		err := testPool.QueryRow(context.Background(),
			`SELECT id FROM organizations WHERE code='gc_test_org' LIMIT 1`).Scan(&orgID)
		if err != nil {
			require.NoError(t, testPool.QueryRow(context.Background(), `
				INSERT INTO organizations (code, name, path, is_virtual, status, sort_order, is_system)
				VALUES ('gc_test_org', 'GC测试', 'gc_test_org', false, 1, 99, false)
				RETURNING id`).Scan(&orgID))
		}
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `UPDATE organizations SET deleted_at=NOW() WHERE code='gc_test_org'`)
		})
		key := "tickets/2026/10/attached.png"
		seedConfirmed(key, "ticket_attachment", 24*30*time.Hour)
		// 建工单+关联行——反连接应排除此对象
		var ticketID int64
		require.NoError(t, testPool.QueryRow(context.Background(), `
			INSERT INTO tickets (type_code, title, status, priority, org_id, org_path, created_by)
			VALUES ('incident', 'gc测试', 'open', 2, $2, 'gc_test_org', $1)
			RETURNING id`, uid, orgID).Scan(&ticketID))
		var foID int64
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT id FROM file_objects WHERE object_key=$1`, key).Scan(&foID))
		_, execErr := testPool.Exec(context.Background(), `
			INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by) VALUES ($1, $2, $3)`,
			ticketID, foID, uid)
		require.NoError(t, execErr)
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_attachments WHERE ticket_id=$1`, ticketID)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM tickets WHERE id=$1`, ticketID)
		})

		require.NoError(t, gc.Handle(context.Background(), nil))
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE object_key=$1`, key).Scan(&status))
		assert.Equal(t, "confirmed", status, "有关联行（在用附件）不得回收")
	})

	// 2026-10-27 G-3 观测批：第三段仍显式豁免 avatar；第四段（avatar 弃置收割）存在但
	// 本实例 observe=true 只记不删——断言语义从「不归第三段」扩为「默认姿态下不删」
	t.Run("非工单域 purpose 不动（avatar 不归第三段；第四段 observe 默认不删）", func(t *testing.T) {
		key := "avatars/gc_exempt.png"
		seedConfirmed(key, "avatar", 24*30*time.Hour)
		require.NoError(t, gc.Handle(context.Background(), nil))
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE object_key=$1`, key).Scan(&status))
		assert.Equal(t, "confirmed", status, "avatar 第三段豁免+第四段 observe 默认不删（enforce 见 G-3 三态用例）")
	})

	// v3 审计 Y-2（2026-10-23）：MarkDeleted 加反连接条件后的并发绑定让位——
	// List 与 Mark 之间被并发建单落绑（插 ticket_attachments 但不改 status）时，
	// MarkDeleted 返回 false（让位），不动物理删（修复前 WHERE 仅守 confirmed → 在用对象被误删）。
	t.Run("并发绑定让位——Mark 返回 false 不物理删（Y-2）", func(t *testing.T) {
		key := "tickets/2026/10/y2-concurrent.png"
		seedConfirmed(key, "attachment_create", 30*24*time.Hour)
		// 模拟竞态：GC List 已完成、MarkDeleted 前——并发建单落绑插关联行（status 仍 confirmed）
		ticketID := insertTicket(t, uid)
		var foID int64
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT id FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, key).Scan(&foID))
		_, execErr := testPool.Exec(context.Background(), `
			INSERT INTO ticket_attachments (ticket_id, file_object_id, uploaded_by) VALUES ($1, $2, $3)`,
			ticketID, foID, uid)
		require.NoError(t, execErr)
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM ticket_attachments WHERE ticket_id=$1`, ticketID)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM tickets WHERE id=$1`, ticketID)
		})
		require.NoError(t, gc.Handle(context.Background(), nil))
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, key).Scan(&status))
		assert.Equal(t, "confirmed", status, "已绑定在用对象不得被第三段软删（Y-2：MarkDeleted NOT EXISTS 让位）")
		assert.NotContains(t, rec.removed, key, "已绑定对象不得被物理删")
	})

	// G-3（2026-10-27 观测批）：avatar 弃置收割三态——observe 不删/enforce 删/bound 不删
	t.Run("avatar 无绑定超期-enforce 模式回收（G-3）", func(t *testing.T) {
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		})
		cfg := &pkgstorage.StorageConfig{
			Enabled: true, Endpoint: "http://stub", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m",
		}
		cfg.ApplyDefaults()
		rec := &removeRecorder{}
		gc := storageservice.NewGCJob(repository.NewStorageRepo(testPool), rec, pkgstorage.NewConfigStore(cfg), testLogger(), false)
		// 隔离：前序子测试残留的 confirmed avatar 会被 enforce 首轮一并收割（行为正确
		// ——弃置即应收；但计数差值断言需只含本用例行）——清桶后种唯一行
		_, err0 := testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
		require.NoError(t, err0)
		key := "avatars/2026/10/abandoned-avatar.png"
		before := middleware.GetCounter("zhuzhao_gc_reclaimed_total", "avatar") // 基线须事前取（进程内全局）
		seedConfirmed(key, "avatar", 24*30*time.Hour)                           // 超 7×2 阈值且无 users.avatar 引用
		require.NoError(t, gc.Handle(context.Background(), nil))
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE object_key=$1`, key).Scan(&status))
		assert.Equal(t, "deleted", status, "enforce 模式下无绑定超期 avatar 应被第四段软删（修复前=永久泄漏）")
		assert.Contains(t, rec.removed, key)
		assert.Equal(t, float64(1), middleware.GetCounter("zhuzhao_gc_reclaimed_total", "avatar")-before, "回收计数按 purpose 入 /metrics")
	})

	t.Run("avatar 已绑定不删-反连接防误删（G-3 防误删守卫）", func(t *testing.T) {
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
			_, _ = testPool.Exec(context.Background(), `UPDATE users SET avatar = '' WHERE id = $1`, uid)
		})
		cfg := &pkgstorage.StorageConfig{
			Enabled: true, Endpoint: "http://stub", Bucket: itBucket,
			AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m",
		}
		cfg.ApplyDefaults()
		rec := &removeRecorder{}
		gc := storageservice.NewGCJob(repository.NewStorageRepo(testPool), rec, pkgstorage.NewConfigStore(cfg), testLogger(), false)
		key := "avatars/2026/10/bound-avatar.png"
		seedConfirmed(key, "avatar", 24*30*time.Hour)
		_, err := testPool.Exec(context.Background(), `UPDATE users SET avatar = $2 WHERE id = $1`, uid, key)
		require.NoError(t, err) // users.avatar 指向该键=在用头像
		require.NoError(t, gc.Handle(context.Background(), nil))
		var status string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT status FROM file_objects WHERE object_key=$1`, key).Scan(&status))
		assert.Equal(t, "confirmed", status, "已绑定 avatar 不得回收（users.avatar 反连接防误删）")
		assert.NotContains(t, rec.removed, key)
	})
}

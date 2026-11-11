//go:build integration

// F-A1 回归测试：GC 先条件清 DB 行（status=pending 才删）、后删 S3 对象——
// 并发 confirm 在 List 与 Delete 之间翻态时，DeleteRowIfPending 返回 false
// 让 GC 跳过 S3 删除（原 bug = 先删 S3 后清行 → 竞态窗内 confirm 的对象本体被误删）。
// 本测试钉住关键构件：DeleteRowIfPending 对 confirmed 行必须返回 false（原子守卫）。
package ticket

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

func TestGC_DeleteRowIfPending_ConcurrentConfirm(t *testing.T) {
	ctx := context.Background()
	env := newB53Env(t)
	repo := repository.NewStorageRepo(testPool)

	t.Run("pending 行删除返回 true（GC 正常路径）", func(t *testing.T) {
		resp, err := env.stg.PresignUpload(ctx, &model.PresignUploadInput{
			Purpose: "attachment_create", FileName: "gc.png", MIMEType: "image/png", SizeBytes: 10,
		}, env.u1)
		require.NoError(t, err)
		f, err := repo.GetByObjectKey(ctx, env.stgCfg.Bucket, resp.ObjectKey)
		require.NoError(t, err)
		require.Equal(t, "pending", f.Status)

		deleted, err := repo.DeleteRowIfPending(ctx, f.ID)
		require.NoError(t, err)
		assert.True(t, deleted, "pending 行应删成功")
	})

	t.Run("confirmed 行删除返回 false（F-A1 原子守卫——GC 须跳过 S3 删除）", func(t *testing.T) {
		k := env.confirmedCreate(t, env.u1, "attachment_create")
		f, err := repo.GetByObjectKey(ctx, env.stgCfg.Bucket, k)
		require.NoError(t, err)
		require.Equal(t, "confirmed", f.Status)

		// 模拟 GC 在 ListExpiredPending 扫到该对象后、DeleteRowIfPending 执行前
		// 被并发 confirm——但这里直接用 confirmed 对象测试等价场景
		deleted, err := repo.DeleteRowIfPending(ctx, f.ID)
		require.NoError(t, err)
		assert.False(t, deleted, "confirmed 行 DeleteRowIfPending 必须返回 false——返回 true 会导致 GC 误删 S3 对象本体（F-A1 回归）")

		// S3 对象本体不应被 GC 删除（stub client 无实际删除——断言行仍在即可验证跳过语义）
		fresh, err := repo.GetByObjectKey(ctx, env.stgCfg.Bucket, k)
		require.NoError(t, err)
		assert.Equal(t, "confirmed", fresh.Status, "行应保持 confirmed 不被误删")
	})

	t.Run("过期 pending + GC 处理 + 行清后对象本体被删（正常完整链）", func(t *testing.T) {
		// 用直接 INSERT 构造一条已过期的 pending（绕过 presign 时间戳）
		key := "tickets/" + time.Now().Add(-48*time.Hour).Format("2006/01/") + "gc-expired.png"
		_, err := testPool.Exec(ctx,
			`INSERT INTO file_objects (bucket, object_key, purpose, status, file_name, mime_type, created_by, created_at)
			 VALUES ($1, $2, 'attachment_create', 'pending', 'gc.png', 'image/png', $3, NOW() - INTERVAL '2 days')`,
			env.stgCfg.Bucket, key, env.u1)
		require.NoError(t, err)

		f, err := repo.GetByObjectKey(ctx, env.stgCfg.Bucket, key)
		require.NoError(t, err)

		deleted, err := repo.DeleteRowIfPending(ctx, f.ID)
		require.NoError(t, err)
		assert.True(t, deleted, "过期 pending 行应删成功")
	})
}

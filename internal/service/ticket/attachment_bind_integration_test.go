//go:build integration

// B5-3 批2：建单前绑定（A.5 建单前链落地点）——测试先行（先红后绿）。
// Create(attachments=[object_key...]) 同事务：校验（本人/confirmed/悬挂态/未绑定）
// → INSERT ticket_attachments → purpose 翻正式态。
// 注：测试文件跨域引用 storageservice 造对象——域隔离守护明示豁免 _test.go。
package ticket

import (
	"context"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	orgdeleg "github.com/tracerbiubiubiu/zhuzhao/internal/service"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
)

// b53Env 建单绑定测试环境：root org + 两个用户 + ticket/storage 双服务。
type b53Env struct {
	svc    *Service
	stg    *storageservice.Service
	stgCfg *pkgstorage.StorageConfig
	u1, u2 int64
	orgID  int64
}

func newB53Env(t *testing.T) *b53Env {
	t.Helper()
	ctx := context.Background()
	ticketRepo := repository.NewTicketRepo(testPool)
	orgRepo := repository.NewOrgRepo(testPool)
	reg := resource.NewRegistry()
	reg.Register(NewResource(ticketRepo, NewPgxScopeResolver(testPool), orgdeleg.NewOrgDelegationService(testPool)))
	stgCfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: "b53",
		AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m"}
	stgCfg.ApplyDefaults()
	svcT := NewTicketService(testPool, ticketRepo, orgRepo, reg, stubRoleFetcher{}, orgdeleg.NewOrgDelegationService(testPool),
		repository.NewStorageRepo(testPool), stgCfg)

	stg := storageservice.NewService(repository.NewStorageRepo(testPool),
		&b53StubClient{info: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}},
		stgCfg, slog.Default())

	env := &b53Env{svc: svcT, stg: stg, stgCfg: stgCfg}
	require.NoError(t, testPool.QueryRow(ctx, `SELECT id FROM organizations WHERE code='root' LIMIT 1`).Scan(&env.orgID))
	for _, u := range []*int64{&env.u1, &env.u2} {
		require.NoError(t, testPool.QueryRow(ctx,
			`INSERT INTO users (employee_no, username, password, status) VALUES ($1, $1, 'hash', 1) RETURNING id`,
			"EB53"+uniqueSuffix()).Scan(u))
		// 挂 root org（Create 归属校验 IsInOrgBranch 前提）
		_, err := testPool.Exec(ctx,
			`INSERT INTO user_orgs (user_id, org_id, is_primary) VALUES ($1, $2, true)`, *u, env.orgID)
		require.NoError(t, err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(ctx, `DELETE FROM file_objects WHERE bucket='b53'`)
		_, _ = testPool.Exec(ctx, `DELETE FROM user_orgs WHERE user_id IN ($1, $2)`, env.u1, env.u2)
		_, _ = testPool.Exec(ctx, `DELETE FROM users WHERE id IN ($1, $2)`, env.u1, env.u2)
	})
	return env
}

// b53StubClient storage.Client 桩（HEAD 实测恒过）。
type b53StubClient struct{ info *pkgstorage.ObjectInfo }

func (c *b53StubClient) PresignPut(_ context.Context, key, ct string, _ time.Duration) (*pkgstorage.PresignedPut, error) {
	return &pkgstorage.PresignedPut{URL: "http://s/" + key, Headers: map[string]string{"Content-Type": ct}}, nil
}
func (c *b53StubClient) PresignGet(_ context.Context, key string, _ time.Duration) (string, error) {
	return "http://s/g/" + key, nil
}
func (c *b53StubClient) StatObject(_ context.Context, _ string) (*pkgstorage.ObjectInfo, error) {
	return c.info, nil
}
func (c *b53StubClient) GetObject(_ context.Context, _, _ string) (io.ReadCloser, *pkgstorage.ObjectInfo, error) {
	return nil, nil, nil
}
func (c *b53StubClient) RemoveObject(_ context.Context, _ string) error { return nil }

// confirmedCreate 建 attachment_create 悬挂 confirmed 对象，返回 object_key。
func (e *b53Env) confirmedCreate(t *testing.T, who int64, purpose string) string {
	t.Helper()
	resp, err := e.stg.PresignUpload(context.Background(), &model.PresignUploadInput{
		Purpose: purpose, FileName: "f.png", MIMEType: "image/png", SizeBytes: 10,
	}, who)
	require.NoError(t, err)
	_, err = e.stg.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: resp.ObjectKey}, who)
	require.NoError(t, err)
	return resp.ObjectKey
}

func (e *b53Env) createReq(keys []string) *model.CreateTicketRequest {
	req := &model.CreateTicketRequest{
		TypeCode: "incident", Title: "B5-3 建单绑定", Priority: 3,
		OrgID: e.orgID, Attachments: keys,
	}
	return req
}

func TestCreate_TicketAttachmentBind(t *testing.T) {
	t.Run("建单落绑——关联行+purpose 翻正式", func(t *testing.T) {
		env := newB53Env(t)
		k1 := env.confirmedCreate(t, env.u1, "attachment_create")
		k2 := env.confirmedCreate(t, env.u1, "richtext_create")

		tk, err := env.svc.Create(context.Background(), env.createReq([]string{k1, k2}), env.u1)
		require.NoError(t, err)

		var n int
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1 AND uploaded_by=$2`, tk.ID, env.u1).Scan(&n))
		assert.Equal(t, 2, n)
		var p1 string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT purpose FROM file_objects WHERE bucket='b53' AND object_key=$1`, k1).Scan(&p1))
		assert.Equal(t, "ticket_attachment", p1, "建单落绑后 purpose 翻正式态")
		var p2 string
		require.NoError(t, testPool.QueryRow(context.Background(),
			`SELECT purpose FROM file_objects WHERE bucket='b53' AND object_key=$1`, k2).Scan(&p2))
		assert.Equal(t, "ticket_richtext", p2)
	})

	t.Run("他人对象拒绝——400 参数错误", func(t *testing.T) {
		env := newB53Env(t)
		k := env.confirmedCreate(t, env.u2, "attachment_create")
		_, err := env.svc.Create(context.Background(), env.createReq([]string{k}), env.u1)
		assert.Equal(t, errcode.ErrInvalidParams.Code, b53Code(t, err))
	})

	t.Run("未确认（pending）对象拒绝", func(t *testing.T) {
		env := newB53Env(t)
		resp, err := env.stg.PresignUpload(context.Background(), &model.PresignUploadInput{
			Purpose: "attachment_create", FileName: "p.png", MIMEType: "image/png", SizeBytes: 10,
		}, env.u1)
		require.NoError(t, err)
		_, err = env.svc.Create(context.Background(), env.createReq([]string{resp.ObjectKey}), env.u1)
		assert.Equal(t, errcode.ErrInvalidParams.Code, b53Code(t, err))
	})

	t.Run("无附件建单——零变化回归", func(t *testing.T) {
		env := newB53Env(t)
		_, err := env.svc.Create(context.Background(), env.createReq(nil), env.u1)
		require.NoError(t, err)
	})

	// S5 评审 P1（2026-10-20）：建单路径个数上限——05 号 §3「单工单附件 ≤20」
	// 在 BindCreateAttachments 的执行点（修复前零执行点）。
	t.Run("建单附件数超上限拒", func(t *testing.T) {
		env := newB53Env(t)
		env.stgCfg.MaxAttachmentsPerTicket = 2
		keys := []string{
			env.confirmedCreate(t, env.u1, "attachment_create"),
			env.confirmedCreate(t, env.u1, "attachment_create"),
			env.confirmedCreate(t, env.u1, "attachment_create"),
		}
		_, err := env.svc.Create(context.Background(), env.createReq(keys), env.u1)
		assert.Equal(t, errcode.ErrInvalidParams.Code, b53Code(t, err), "3 keys 超上限 2 应整单拒绝")
	})

	// S5 评审 P1（2026-10-20）：并发双绑窗口——两个并发 Create 携同一悬挂对象，
	// 应恰一成功（修复前双双成功、一对象绑两单）。
	t.Run("并发建单同对象恰一成功", func(t *testing.T) {
		env := newB53Env(t)
		k := env.confirmedCreate(t, env.u1, "attachment_create")

		type bindRes struct {
			id  int64
			err error
		}
		ch := make(chan bindRes, 2)
		for i := 0; i < 2; i++ {
			go func() {
				tk, err := env.svc.Create(context.Background(), env.createReq([]string{k}), env.u1)
				var id int64
				if tk != nil {
					id = tk.ID
				}
				ch <- bindRes{id: id, err: err}
			}()
		}
		r1, r2 := <-ch, <-ch
		successes := 0
		for i, r := range []bindRes{r1, r2} {
			if r.err == nil {
				successes++
				continue
			}
			assert.Equal(t, errcode.ErrInvalidParams.Code, b53Code(t, r.err),
				"失败侧应为 400 无效对象（第 %d 个并发）", i+1)
		}
		assert.Equal(t, 1, successes, "并发双绑应恰一成功")

		var n int
		require.NoError(t, testPool.QueryRow(context.Background(), `
			SELECT count(*) FROM ticket_attachments ta
			JOIN file_objects fo ON fo.id = ta.file_object_id
			WHERE fo.object_key = $1`, k).Scan(&n))
		assert.Equal(t, 1, n, "对象只能落一个工单的关联行")
	})
}

func b53Code(t *testing.T, err error) int {
	t.Helper()
	var e *errcode.Error
	require.ErrorAs(t, err, &e)
	return e.Code
}

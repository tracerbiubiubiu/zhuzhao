//go:build integration

// C04 撤权提交顺序（票态附件面）专用回归。守卫语义与工单 update/transition
// 同款（internal/service/ticket_c04_integration_test.go 为准入面权威）：
//   - confirm/delete 在 IAM 共同共享守卫内锁票、同连接重认证——撤销先提交
//     则拒绝（confirm 零绑定 / delete 不发生），命令先完成则写入保留；
//   - HEAD 实测（外部动作）留在 IAM 锁外；
//   - 直连调用（无 HTTP 准入 purpose）维持既有契约，守卫不接管；
//   - C02 历史引用保护在守卫路径内继续拦截。
//
// 单连接池 + 查询追踪实证守卫全程不复用第二个池连接（防二次取池死锁）。
package storage_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/casbin/casbin/v3"
	casbinmodel "github.com/casbin/casbin/v3/model"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tracerbiubiubiu/zhuzhao/internal/handler"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/resource"
	pkgstorage "github.com/tracerbiubiubiu/zhuzhao/internal/pkg/storage"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
	"github.com/tracerbiubiubiu/zhuzhao/internal/service"
	storageservice "github.com/tracerbiubiubiu/zhuzhao/internal/service/storage"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// attachmentCommandFixture 活跃角色 + 属主工单最小装置：守卫要求活跃身份与
// 当前角色（len(roles)>0），L2 update 面走属主判定。
func attachmentCommandFixture(t *testing.T) (actor, ticketID int64, roleCode string) {
	t.Helper()
	roleCode = "c04attach_" + uniqueSuffix()
	actor = insertUser(t)
	var role int64
	require.NoError(t, testPool.QueryRow(context.Background(),
		`INSERT INTO roles(code,name,priority)VALUES($1::text,$1::text,50)RETURNING id`, roleCode).Scan(&role))
	_, err := testPool.Exec(context.Background(),
		`INSERT INTO user_roles(user_id,role_id)VALUES($1,$2)`, actor, role)
	require.NoError(t, err)
	ticketID = insertTicket(t, actor)
	return actor, ticketID, roleCode
}

// revokeRoles 撤销经独占 IAM 守卫真实提交（与生产撤销写入同路径），非测试桩。
func revokeRoles(t *testing.T, userID int64) {
	t.Helper()
	require.NoError(t, repository.WithIAMGuard(context.Background(), testPool, func(ctx context.Context) error {
		_, err := repository.IAMDB(ctx, testPool).Exec(ctx, `DELETE FROM user_roles WHERE user_id=$1`, userID)
		return err
	}))
}

// guardBundle 同池的 ticket+storage 服务对（守卫拒绝跨池）+真实 handler
// （构造器能力探测注入守卫）。
type guardBundle struct {
	storage *storageservice.Service
	ticket  *ticketsvc.Service
	handler *handler.StorageHandler
}

func newGuardBundle(t *testing.T, pool *pgxpool.Pool, client pkgstorage.Client) *guardBundle {
	t.Helper()
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM file_objects WHERE bucket = $1`, itBucket)
	})
	cfg := &pkgstorage.StorageConfig{Enabled: true, Endpoint: "s", Bucket: itBucket,
		AccessKey: "ak", SecretKey: "sk", PresignUploadTTL: "15m"}
	cfg.ApplyDefaults()
	storageSvc := storageservice.NewService(repository.NewStorageRepo(pool), client, pkgstorage.NewConfigStore(cfg), testLogger())
	m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	rbac := service.NewRBACService(repository.NewRoleRepo(pool), repository.NewUserRepo(pool), repository.NewMenuRepo(pool), enforcer)
	ticketSvc := ticketsvc.NewTicketService(pool, repository.NewTicketRepo(pool), repository.NewOrgRepo(pool),
		resource.NewRegistry(), rbac, service.NewOrgDelegationService(pool), nil, nil, nil)
	h := handler.NewStorageHandler(storageSvc, ticketSvc, nil)
	return &guardBundle{storage: storageSvc, ticket: ticketSvc, handler: h}
}

// mintPurpose 经真实 SelfService→CasbinAuth 准入链铸造 purpose ctx——守卫拒绝
// 伪造，只有成功准入的请求才携带 IAMRequestPurpose。
func mintPurpose(t *testing.T, actor int64, roleCode, path string, selfService bool) context.Context {
	t.Helper()
	m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	if !selfService {
		_, err = enforcer.AddPolicy("role::"+roleCode, path, http.MethodPost)
		require.NoError(t, err)
	}
	rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool),
		repository.NewMenuRepo(testPool), enforcer)
	var minted context.Context
	r := gin.New()
	chain := []gin.HandlerFunc{
		func(c *gin.Context) {
			c.Set("userID", actor)
			c.Set("session_epoch", int64(1)) // 初始数据库安全代际的合成准入事实
			c.Next()
		},
	}
	if selfService {
		chain = append(chain, middleware.SelfService())
	}
	chain = append(chain, middleware.CasbinAuth(enforcer, rbac, nil),
		func(c *gin.Context) { minted = c.Request.Context() })
	r.POST(path, chain...)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest(http.MethodPost, path, nil))
	require.NotNil(t, minted, "准入链须铸造 purpose ctx")
	return minted
}

// revokingStatClient HEAD 实测窗口注入撤销提交——确定性复现「handler 前置
// L2 已过、撤销落库、守卫未进」的真实竞态窗（测试合成，无并发抖动）。
type revokingStatClient struct {
	stubClient
	headGuardActive bool
	beforeStat      func() error
}

func (c *revokingStatClient) StatObject(ctx context.Context, key string) (*pkgstorage.ObjectInfo, error) {
	c.headGuardActive = repository.IAMGuardActive(ctx)
	if c.beforeStat != nil {
		if err := c.beforeStat(); err != nil {
			return nil, err
		}
	}
	return c.stubClient.StatObject(ctx, key)
}

// attachmentCommandTrace 守卫内查询连接观察（生产查询执行，非授权桩）。
type attachmentCommandTrace struct {
	mu   sync.Mutex
	pids map[uint32]bool
}

func (tr *attachmentCommandTrace) TraceQueryStart(ctx context.Context, conn *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	if repository.IAMGuardActive(ctx) {
		tr.mu.Lock()
		tr.pids[conn.PgConn().PID()] = true
		tr.mu.Unlock()
	}
	return ctx
}
func (*attachmentCommandTrace) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func singleConnPool(t *testing.T) (*pgxpool.Pool, *attachmentCommandTrace) {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(testPool.Config().ConnString())
	require.NoError(t, err)
	cfg.MaxConns = 1
	trace := &attachmentCommandTrace{pids: make(map[uint32]bool)}
	cfg.ConnConfig.Tracer = trace
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(pool.Close)
	return pool, trace
}

func attachmentCount(t *testing.T, ticketID int64) int {
	t.Helper()
	var n int
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM ticket_attachments WHERE ticket_id=$1`, ticketID).Scan(&n))
	return n
}

func objectStatus(t *testing.T, key string) string {
	t.Helper()
	var status string
	require.NoError(t, testPool.QueryRow(context.Background(),
		`SELECT status FROM file_objects WHERE bucket=$1 AND object_key=$2`, itBucket, key).Scan(&status))
	return status
}

func presignTicketAttachment(t *testing.T, svc *storageservice.Service, userID, ticketID int64) string {
	t.Helper()
	resp, err := svc.PresignUpload(context.Background(), &model.PresignUploadInput{
		Purpose: "ticket_attachment", FileName: "附.png", MIMEType: "image/png", SizeBytes: 10, TicketID: &ticketID,
	}, userID)
	require.NoError(t, err)
	return resp.ObjectKey
}

func TestAttachmentCommand_Presign(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		t.Run(fmt.Sprint("after_revocation=", revoke), func(t *testing.T) {
			const path = "/api/v1/storage/presign/upload"
			actor, ticketID, code := attachmentCommandFixture(t)
			pool, trace := singleConnPool(t)
			b := newGuardBundle(t, pool, &stubClient{})
			ctx := mintPurpose(t, actor, code, path, true)
			if revoke {
				revokeRoles(t, actor)
			}
			ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()
			resp, err := b.storage.PresignUpload(ctx, &model.PresignUploadInput{
				Purpose: " ticket_attachment ", FileName: "draft.png", MIMEType: "image/png", SizeBytes: 10, TicketID: &ticketID,
			}, actor)
			var pending int
			require.NoError(t, testPool.QueryRow(ctx, `SELECT count(*) FROM file_objects WHERE bucket=$1 AND created_by=$2 AND status='pending'`, itBucket, actor).Scan(&pending))
			require.Len(t, trace.pids, 1)
			require.Zero(t, pool.Stat().CanceledAcquireCount())
			if revoke {
				require.Error(t, err)
				require.Equal(t, errcode.ErrNoRoles.Code, codeOf(t, err))
				require.Nil(t, resp)
				require.Zero(t, pending)
				return
			}
			require.NoError(t, err)
			require.NotEmpty(t, resp.UploadURL)
			require.Equal(t, 1, pending)
		})
	}
}

func TestAttachmentCommand_Confirm(t *testing.T) {
	const confirmPath = "/api/v1/tickets/attachments/confirm"

	t.Run("撤销先提交——守卫内重认证拒绝且零绑定", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := presignTicketAttachment(t, b.storage, actor, tkID)
		// 准入（handler 前置 L2 + L1 purpose 铸造）时角色在场，撤销随后提交。
		require.NoError(t, b.ticket.AuthorizeTicketAttachment(context.Background(), actor, tkID))
		ctx := mintPurpose(t, actor, roleCode, confirmPath, true)
		revokeRoles(t, actor)

		_, err := b.storage.Confirm(ctx,
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.Error(t, err, "守卫内重认证必须拒绝撤销后命令")
		assert.Equal(t, errcode.ErrNoRoles.Code, codeOf(t, err))
		assert.Zero(t, attachmentCount(t, tkID), "撤销先提交——零绑定")
		assert.Equal(t, model.FileStatusPending, objectStatus(t, key), "对象保持 pending 不落绑")
	})

	t.Run("HEAD 窗口撤销——准入后守卫前落库仍拒绝且 HEAD 在锁外", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		stub := &revokingStatClient{stubClient: stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}}
		stub.beforeStat = func() error { revokeRoles(t, actor); return nil }
		b := newGuardBundle(t, testPool, stub)
		key := presignTicketAttachment(t, b.storage, actor, tkID)
		require.NoError(t, b.ticket.AuthorizeTicketAttachment(context.Background(), actor, tkID))

		_, err := b.storage.Confirm(mintPurpose(t, actor, roleCode, confirmPath, true),
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.Error(t, err, "准入与守卫之间的撤销窗口必须被关闭")
		assert.Equal(t, errcode.ErrNoRoles.Code, codeOf(t, err))
		assert.Zero(t, attachmentCount(t, tkID), "HEAD 窗口撤销——零绑定")
		assert.False(t, stub.headGuardActive, "HEAD 实测（外部动作）不得在 IAM 锁内执行")
	})

	t.Run("命令先完成——绑定保留不被追溯撤销", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := presignTicketAttachment(t, b.storage, actor, tkID)

		f, err := b.storage.Confirm(mintPurpose(t, actor, roleCode, confirmPath, true),
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.NoError(t, err, "命令先完成——合法提交")
		assert.Equal(t, model.FileStatusConfirmed, f.Status)
		require.Equal(t, 1, attachmentCount(t, tkID))

		revokeRoles(t, actor)
		assert.Equal(t, 1, attachmentCount(t, tkID), "已提交命令不随后续撤销回滚")
		assert.Equal(t, model.FileStatusConfirmed, objectStatus(t, key))
	})

	t.Run("合法路径——单连接守卫提交", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		pool, trace := singleConnPool(t)
		b := newGuardBundle(t, pool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := presignTicketAttachment(t, b.storage, actor, tkID)

		f, err := b.storage.Confirm(mintPurpose(t, actor, roleCode, confirmPath, true),
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusConfirmed, f.Status)
		assert.Equal(t, 1, attachmentCount(t, tkID))
		assert.Len(t, trace.pids, 1, "守卫内 IAM 新鲜读取、L2 与写入共用一个连接")
		assert.Zero(t, pool.Stat().CanceledAcquireCount(), "授权与写入不得二次取池连接")
	})

	t.Run("wrong_actor purpose 拒", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		other := insertUser(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		// 对象归属 other：先过归属面，验证 purpose.UserID 与执行者不一致被拒。
		key := presignTicketAttachment(t, b.storage, other, tkID)

		_, err := b.storage.Confirm(mintPurpose(t, actor, roleCode, confirmPath, true),
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, other)
		require.Error(t, err)
		assert.Equal(t, errcode.ErrNoPermission.Code, codeOf(t, err))
		assert.Zero(t, attachmentCount(t, tkID))
	})

	t.Run("非自服务 purpose 拒（fail-closed）", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := presignTicketAttachment(t, b.storage, actor, tkID)

		_, err := b.storage.Confirm(mintPurpose(t, actor, roleCode, confirmPath, false),
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.Error(t, err, "票态附件端点登记在 selfService 组——非自服务 purpose 一律拒绝")
		assert.Equal(t, errcode.ErrNoPermission.Code, codeOf(t, err))
		assert.Zero(t, attachmentCount(t, tkID))
	})

	t.Run("跨路由 purpose 拒", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := presignTicketAttachment(t, b.storage, actor, tkID)

		// confirm 命令只接受 confirm 路由铸造的 purpose。
		_, err := b.storage.Confirm(mintPurpose(t, actor, roleCode, "/api/v1/tickets/attachments/delete", true),
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.Error(t, err)
		assert.Equal(t, errcode.ErrNoPermission.Code, codeOf(t, err))
		assert.Zero(t, attachmentCount(t, tkID))
	})

	t.Run("直连调用无 HTTP 准入——守卫不接管（既有契约）", func(t *testing.T) {
		actor, tkID, _ := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := presignTicketAttachment(t, b.storage, actor, tkID)

		// 非 HTTP ctx：既有直连调用方契约不变（守卫仅接管已准入 HTTP 命令）。
		f, err := b.storage.Confirm(context.Background(),
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.NoError(t, err)
		assert.Equal(t, model.FileStatusConfirmed, f.Status)
		assert.Equal(t, 1, attachmentCount(t, tkID))
	})

	t.Run("账号停用——守卫内拒绝", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := presignTicketAttachment(t, b.storage, actor, tkID)
		// 准入先通过（角色在场），停用随后经 IAM 守卫提交。
		ctx := mintPurpose(t, actor, roleCode, confirmPath, true)
		require.NoError(t, repository.WithIAMGuard(context.Background(), testPool, func(gctx context.Context) error {
			_, err := repository.IAMDB(gctx, testPool).Exec(gctx, `UPDATE users SET status=0 WHERE id=$1`, actor)
			return err
		}))

		_, err := b.storage.Confirm(ctx,
			&model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.Error(t, err)
		assert.Equal(t, errcode.ErrNoPermission.Code, codeOf(t, err))
		assert.Zero(t, attachmentCount(t, tkID))
	})
}

func TestAttachmentCommand_Delete(t *testing.T) {
	const deletePath = "/api/v1/tickets/attachments/delete"

	// boundGuarded 走守卫已接线的服务在非 HTTP ctx 下建绑（直连契约——装置步骤）。
	boundGuarded := func(t *testing.T, b *guardBundle, actor, tkID int64) string {
		t.Helper()
		key := presignTicketAttachment(t, b.storage, actor, tkID)
		_, err := b.storage.Confirm(context.Background(), &model.ConfirmFileInput{ObjectKey: key, TicketID: &tkID}, actor)
		require.NoError(t, err)
		return key
	}

	t.Run("撤销先提交——删除不发生", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := boundGuarded(t, b, actor, tkID)
		// 准入（handler 前置 L2 + L1 purpose 铸造）时角色在场，撤销随后提交。
		require.NoError(t, b.ticket.AuthorizeTicketAttachment(context.Background(), actor, tkID))
		ctx := mintPurpose(t, actor, roleCode, deletePath, true)
		revokeRoles(t, actor)

		err := b.storage.DeleteTicketAttachmentAs(ctx, actor, tkID, key)
		require.Error(t, err, "守卫内重认证必须拒绝撤销后命令")
		assert.Equal(t, errcode.ErrNoRoles.Code, codeOf(t, err))
		assert.Equal(t, 1, attachmentCount(t, tkID), "撤销先提交——关联行保留")
		assert.Equal(t, model.FileStatusConfirmed, objectStatus(t, key), "对象不标 deleted")
	})

	t.Run("命令先完成——删除生效保留", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := boundGuarded(t, b, actor, tkID)

		require.NoError(t, b.storage.DeleteTicketAttachmentAs(mintPurpose(t, actor, roleCode, deletePath, true), actor, tkID, key))
		assert.Zero(t, attachmentCount(t, tkID))
		assert.Equal(t, model.FileStatusDeleted, objectStatus(t, key))

		revokeRoles(t, actor)
		assert.Zero(t, attachmentCount(t, tkID), "已提交命令不随后续撤销回滚")
	})

	t.Run("合法路径——单连接守卫提交+物理删在提交后", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		pool, trace := singleConnPool(t)
		stub := &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}}
		b := newGuardBundle(t, pool, stub)
		key := boundGuarded(t, b, actor, tkID)

		require.NoError(t, b.storage.DeleteTicketAttachmentAs(mintPurpose(t, actor, roleCode, deletePath, true), actor, tkID, key))
		assert.Zero(t, attachmentCount(t, tkID))
		assert.Equal(t, model.FileStatusDeleted, objectStatus(t, key))
		assert.Equal(t, []string{key}, stub.removedKeys(), "物理对象删除在 DB 提交后 best-effort 执行")
		assert.Len(t, trace.pids, 1, "守卫内 IAM 新鲜读取、L2 与写入共用一个连接")
		assert.Zero(t, pool.Stat().CanceledAcquireCount(), "授权与写入不得二次取池连接")
	})

	t.Run("C02 历史引用保护在守卫路径内保留", func(t *testing.T) {
		actor, tkID, roleCode := attachmentCommandFixture(t)
		b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
		key := boundGuarded(t, b, actor, tkID)
		// 未知历史字段（incident 类型无字段定义）内嵌对象键——C02：引用即保护字节。
		_, err := testPool.Exec(context.Background(),
			`UPDATE tickets SET custom_data=$2::jsonb WHERE id=$1`,
			tkID, fmt.Sprintf(`{"legacy_note":"see %s"}`, key))
		require.NoError(t, err)

		err = b.storage.DeleteTicketAttachmentAs(mintPurpose(t, actor, roleCode, deletePath, true), actor, tkID, key)
		require.Error(t, err, "历史引用保护必须先于删除生效")
		assert.Equal(t, errcode.ErrConflict.Code, codeOf(t, err))
		assert.Equal(t, 1, attachmentCount(t, tkID), "被引用附件关联行保留")
		assert.Equal(t, model.FileStatusConfirmed, objectStatus(t, key), "被引用附件不标 deleted")
	})
}

// TestAttachmentCommand_HandlerE2E 真实 gin 链（SelfService→CasbinAuth→handler）
// 全栈：构造器能力探测注入守卫后合法路径 200，撤销后 403/404 面拒绝。
func TestAttachmentCommand_HandlerE2E(t *testing.T) {
	const confirmPath = "/api/v1/tickets/attachments/confirm"
	actor, tkID, _ := attachmentCommandFixture(t)
	b := newGuardBundle(t, testPool, &stubClient{stat: &pkgstorage.ObjectInfo{Size: 10, ETag: "e", ContentType: "image/png"}})
	key := presignTicketAttachment(t, b.storage, actor, tkID)

	m, err := casbinmodel.NewModelFromFile("../../../configs/casbin_model.conf")
	require.NoError(t, err)
	enforcer, err := casbin.NewSyncedEnforcer(m)
	require.NoError(t, err)
	rbac := service.NewRBACService(repository.NewRoleRepo(testPool), repository.NewUserRepo(testPool),
		repository.NewMenuRepo(testPool), enforcer)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		c.Set("userID", actor)
		c.Set("session_epoch", int64(1))
		c.Next()
	},
		middleware.SelfService(), middleware.CasbinAuth(enforcer, rbac, nil))
	r.POST(confirmPath, b.handler.Confirm)
	r.POST("/api/v1/tickets/attachments/delete", b.handler.DeleteAttachment)

	// 合法路径：守卫放行 → 落绑 200。
	reqCtx, reqCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer reqCancel()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, confirmPath,
		strings.NewReader(fmt.Sprintf(`{"object_key":%q,"ticket_id":"%d"}`, key, tkID))).WithContext(reqCtx)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusOK, w.Code, w.Body.String())
	assert.Equal(t, 1, attachmentCount(t, tkID))

	// 撤销后同链删除请求：L1 零角色即拒（准入先于 handler），关联行保留。
	revokeRoles(t, actor)
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/v1/tickets/attachments/delete",
		strings.NewReader(fmt.Sprintf(`{"ticket_id":"%d","object_key":%q}`, tkID, key))).WithContext(reqCtx)
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusForbidden, w.Code, w.Body.String())
	assert.Equal(t, 1, attachmentCount(t, tkID), "准入拒绝先行——关联行保留")
}

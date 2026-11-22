//go:build integration

package admincli_test

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	goredis "github.com/redis/go-redis/v9"

	"github.com/tracerbiubiubiu/zhuzhao-utils/crypto"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/admincli"
	"github.com/tracerbiubiubiu/zhuzhao/internal/testutil"
)

var (
	testPool *pgxpool.Pool
	testRDB  *goredis.Client
)

func TestMain(m *testing.M) {
	pool, cleanup, err := testutil.SetupPostgresShared()
	if err != nil {
		panic(err)
	}
	testPool = pool

	// 测试基建有意排除 000002 种子（避免幻影行）——superadmin 角色自建
	//（幂等；users 由各用例自建，勿共用）。
	if _, err := pool.Exec(context.Background(), `
		INSERT INTO roles (code, name, description, priority, is_system)
		SELECT 'superadmin', '超级管理员', 'break-glass 测试角色', 1, true
		WHERE NOT EXISTS (SELECT 1 FROM roles WHERE code = 'superadmin' AND deleted_at IS NULL)`); err != nil {
		panic(err)
	}

	mr, err := miniredis.Run()
	if err != nil {
		panic(err)
	}
	testRDB = goredis.NewClient(&goredis.Options{Addr: mr.Addr()})

	code := m.Run()
	_ = testRDB.Close()
	mr.Close()
	cleanup()
	os.Exit(code)
}

func opts(username, password string) admincli.ResetOptions {
	return admincli.ResetOptions{
		Username: username, Password: password,
		Note: "integration drill", Pool: testPool, RDB: testRDB,
		AccessTTL: time.Minute,
	}
}

func queryInt(t *testing.T, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if err := testPool.QueryRow(context.Background(), q, args...).Scan(&n); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return n
}

func queryString(t *testing.T, q string, args ...any) string {
	t.Helper()
	var s string
	if err := testPool.QueryRow(context.Background(), q, args...).Scan(&s); err != nil {
		t.Fatalf("query %q: %v", q, err)
	}
	return s
}

func uniqueUser(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("bg_drill_%d", time.Now().UnixNano())
}

// createSuperadminUser 自建持 superadmin 角色的用户（status 可置 0 模拟禁用态）。
func createSuperadminUser(t *testing.T, status int) int64 {
	t.Helper()
	ctx := context.Background()
	var id int64
	if err := testPool.QueryRow(ctx,
		`INSERT INTO users (username, password, employee_no, real_name, status) VALUES ($1, 'hash', $2, 'x', $3) RETURNING id`,
		uniqueUser(t), "EBG_"+strconv.FormatInt(time.Now().UnixNano(), 10), status).Scan(&id); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	if _, err := testPool.Exec(ctx,
		`INSERT INTO user_roles (user_id, role_id) SELECT $1, id FROM roles WHERE code = 'superadmin' AND deleted_at IS NULL`,
		id); err != nil {
		t.Fatalf("bind role: %v", err)
	}
	return id
}

// EndToEnd：自建 superadmin 用户锁死→恢复全链断言——密码置换/强制改密/
// 会话吊销（数据库代际推进、Redis 确认、旧代刷新槽隔离）和审计留痕。
func TestResetSuperadminPassword_EndToEnd(t *testing.T) {
	ctx := context.Background()

	userID := createSuperadminUser(t, 1)
	oldHash := queryString(t, `SELECT password FROM users WHERE id = $1`, userID)

	oldEpoch := queryInt(t, `SELECT session_epoch FROM users WHERE id = $1`, userID)
	refreshKey := fmt.Sprintf("refresh:db1:%d:%d:drill-device", userID, oldEpoch)
	if err := testRDB.Set(ctx, refreshKey, "old-slot-digest", time.Minute).Err(); err != nil {
		t.Fatalf("seed refresh key: %v", err)
	}

	res, err := admincli.ResetSuperadminPassword(ctx, opts(queryString(t,
		`SELECT username FROM users WHERE id = $1`, userID), "Drill-Passw0rd!"))
	if err != nil {
		t.Fatalf("reset: %v", err)
	}

	if res.UserID != userID {
		t.Fatalf("result target mismatch: %+v", res)
	}
	if !res.ForcedChange || !res.SessionsRevoked || !res.AuditWritten || res.Reenabled {
		t.Fatalf("unexpected flags: %+v", res)
	}

	// DB：hash 已置换且可登录比对；must_change_password=true
	newHash := queryString(t, `SELECT password FROM users WHERE id = $1`, userID)
	if newHash == oldHash {
		t.Fatal("password hash 未变化")
	}
	if !crypto.CheckPassword("Drill-Passw0rd!", newHash) {
		t.Fatal("新 hash 与新密码比对失败")
	}
	if n := queryInt(t, `SELECT COUNT(*) FROM users WHERE id = $1 AND must_change_password`, userID); n != 1 {
		t.Fatal("must_change_password 应为 true")
	}

	// 旧槽按自己的 TTL 到期。撤销靠代际推进，避免异步清理波及新登录。
	newEpoch := queryInt(t, `SELECT session_epoch FROM users WHERE id = $1`, userID)
	if newEpoch != oldEpoch+1 {
		t.Fatalf("DB session_epoch = %d, want %d", newEpoch, oldEpoch+1)
	}
	epochKey := fmt.Sprintf("user:session_epoch:db1:%d", userID)
	projected, err := testRDB.Get(ctx, epochKey).Int64()
	if err != nil || projected != newEpoch {
		t.Fatalf("Redis session epoch = %d (err=%v), want %d", projected, err, newEpoch)
	}
	if confirmed := queryInt(t, `SELECT session_projection_epoch FROM users WHERE id = $1`, userID); confirmed != newEpoch {
		t.Fatalf("DB projection epoch = %d, want %d", confirmed, newEpoch)
	}
	newRefreshKey := fmt.Sprintf("refresh:db1:%d:%d:drill-device", userID, newEpoch)
	if got := testRDB.Exists(ctx, newRefreshKey).Val(); got != 0 {
		t.Fatalf("reset must not issue refresh state for the new epoch, got %d", got)
	}

	// 审计：CLI 行落 audit_logs
	if n := queryInt(t,
		`SELECT COUNT(*) FROM audit_logs WHERE method = 'CLI' AND user_id = $1 AND path = 'break-glass:reset-password'`,
		userID); n != 1 {
		t.Fatalf("audit_logs CLI 行数 = %d, want 1", n)
	}
}

// 非 superadmin 目标拒绝（普通用户必须走管理端重置，不得绕层级防护）。
func TestResetSuperadminPassword_NonSuperadminRejected(t *testing.T) {
	ctx := context.Background()
	name := uniqueUser(t)
	if _, err := testPool.Exec(ctx,
		`INSERT INTO users (username, password, employee_no, real_name, status) VALUES ($1, 'hash', $2, 'x', 1)`,
		name, "EBG_"+strconv.FormatInt(time.Now().UnixNano(), 10)); err != nil {
		t.Fatalf("insert user: %v", err)
	}
	_, err := admincli.ResetSuperadminPassword(ctx, opts(name, "Drill-Passw0rd!"))
	if !errors.Is(err, admincli.ErrTargetNotSuperadmin) {
		t.Fatalf("err = %v, want ErrTargetNotSuperadmin", err)
	}
}

// 禁用态 superadmin 连带启用（锁死叠加场景）。
func TestResetSuperadminPassword_DisabledSuperadminReenabled(t *testing.T) {
	ctx := context.Background()
	id := createSuperadminUser(t, 0)
	username := queryString(t, `SELECT username FROM users WHERE id = $1`, id)

	res, err := admincli.ResetSuperadminPassword(ctx, opts(username, "Drill-Passw0rd!"))
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if !res.Reenabled {
		t.Fatalf("Reenabled = false, want true: %+v", res)
	}
	if n := queryInt(t, `SELECT COUNT(*) FROM users WHERE id = $1 AND status = 1`, id); n != 1 {
		t.Fatal("用户应已启用")
	}
	if res.PendingReason != "" {
		t.Fatalf("healthy re-enable unexpectedly pending: %s", res.PendingReason)
	}
	if n := queryInt(t, `SELECT COUNT(*) FROM session_projection_intents
		WHERE user_id = $1 AND status = 'confirmed' AND
		((reason = 'cli-reset-password' AND NOT desired_active) OR (reason = 'cli-enable' AND desired_active))`, id); n != 2 {
		t.Fatalf("confirmed reset and enable intents = %d, want 2", n)
	}
}

func TestResetSuperadminPassword_DisabledStaysDisabledWhileProjectionPending(t *testing.T) {
	ctx := context.Background()
	id := createSuperadminUser(t, 0)
	username := queryString(t, `SELECT username FROM users WHERE id = $1`, id)
	rdb := goredis.NewClient(&goredis.Options{
		MaxRetries: -1,
		Dialer: func(context.Context, string, string) (net.Conn, error) {
			return nil, errors.New("isolated Redis unavailable")
		},
	})
	t.Cleanup(func() { _ = rdb.Close() })
	options := opts(username, "Drill-Passw0rd!")
	options.RDB = rdb

	res, err := admincli.ResetSuperadminPassword(ctx, options)
	if err != nil {
		t.Fatalf("reset with pending projection: %v", err)
	}
	if res.OperationID == "" || res.PendingReason == "" || res.Reenabled || res.SessionsRevoked || !res.AuditWritten {
		t.Fatal("expected an auditable pending reset and no re-enable")
	}
	if n := queryInt(t, `SELECT COUNT(*) FROM users WHERE id = $1 AND status = 0
		AND session_epoch = 2 AND session_projection_epoch = 0 AND must_change_password`, id); n != 1 {
		t.Fatal("reset must commit and preserve the disabled account while projection is pending")
	}
	if n := queryInt(t, `SELECT COUNT(*) FROM session_projection_intents
		WHERE user_id = $1 AND NOT desired_active AND status = 'pending' AND reason = 'cli-reset-password'`, id); n != 1 {
		t.Fatal("expected the original pending reset intent")
	}
	if n := queryInt(t, `SELECT COUNT(*) FROM session_projection_intents
		WHERE user_id = $1 AND reason = 'cli-enable'`, id); n != 0 {
		t.Fatal("pending reset must not generate an enable intent")
	}
}

// 目标不存在。
func TestResetSuperadminPassword_MissingTarget(t *testing.T) {
	_, err := admincli.ResetSuperadminPassword(context.Background(),
		opts("bg_ghost_"+strconv.FormatInt(time.Now().UnixNano(), 10), "Drill-Passw0rd!"))
	if !errors.Is(err, admincli.ErrTargetNotFound) {
		t.Fatalf("err = %v, want ErrTargetNotFound", err)
	}
}

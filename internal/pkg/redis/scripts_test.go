package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

func newTestScripts(t *testing.T) (*Scripts, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	client := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })
	return NewScripts(client), mr
}

// D2-45：登录锁定 Lua 阈值语义——第 1~5 次失败不锁定（返回 0），
// 第 6 次起返回 1（blocked）。阈值边界（=5 与 >5）从未被测试验证过。
func TestLoginLockIncr_Threshold(t *testing.T) {
	s, _ := newTestScripts(t)
	ctx := context.Background()

	for i := 1; i <= loginLockMaxFail; i++ {
		blocked, err := s.LoginLockIncr(ctx, "E900001")
		if err != nil {
			t.Fatalf("incr #%d: %v", i, err)
		}
		if blocked {
			t.Fatalf("第 %d 次失败（≤ 阈值 %d）不应锁定", i, loginLockMaxFail)
		}
	}
	// 第 6 次（n=6 > 5）→ blocked
	blocked, err := s.LoginLockIncr(ctx, "E900001")
	if err != nil {
		t.Fatal(err)
	}
	if !blocked {
		t.Fatal("超过阈值后应返回 blocked=true")
	}
	// IsBlocked 视图一致（count > max_fail）
	is, err := s.LoginLockIsBlocked(ctx, "E900001")
	if err != nil || !is {
		t.Fatalf("IsBlocked = %v, %v；want true, nil", is, err)
	}
}

// D2-45：EXPIRE 时机——仅首次 INCR 设置 TTL（窗口 15min），
// 后续失败不重置窗口（防「持续爆破刷新窗口」绕过）。
func TestLoginLockIncr_ExpireOnlyOnFirst(t *testing.T) {
	s, mr := newTestScripts(t)
	ctx := context.Background()
	const emp = "E900002"

	if _, err := s.LoginLockIncr(ctx, emp); err != nil {
		t.Fatal(err)
	}
	ttl1 := mr.TTL(loginLockKeyPrefix + emp)
	if ttl1 <= 0 {
		t.Fatalf("首次失败后应有 TTL，实际 %v", ttl1)
	}
	if ttl1 > loginLockWindowSec*time.Second {
		t.Fatalf("TTL %v 超过窗口 %v", ttl1, loginLockWindowSec*time.Second)
	}

	// 快进接近窗口末尾后再失败一次——若错误地重置 EXPIRE，TTL 会回到完整窗口
	mr.FastForward(loginLockWindowSec*time.Second - 2*time.Second)
	if _, err := s.LoginLockIncr(ctx, emp); err != nil {
		t.Fatal(err)
	}
	ttl2 := mr.TTL(loginLockKeyPrefix + emp)
	if ttl2 > 3*time.Second {
		t.Fatalf("后续失败不应重置窗口：TTL=%v（应接近 2s）", ttl2)
	}
}

// D2-45：登录成功清零计数（锁定恢复入口）
func TestLoginLockClear(t *testing.T) {
	s, _ := newTestScripts(t)
	ctx := context.Background()
	const emp = "E900003"

	for i := 0; i < loginLockMaxFail+1; i++ {
		_, _ = s.LoginLockIncr(ctx, emp)
	}
	if is, _ := s.LoginLockIsBlocked(ctx, emp); !is {
		t.Fatal("前置：应处于锁定态")
	}
	if err := s.LoginLockClear(ctx, emp); err != nil {
		t.Fatal(err)
	}
	if is, err := s.LoginLockIsBlocked(ctx, emp); err != nil || is {
		t.Fatalf("清零后 IsBlocked = %v, %v；want false, nil", is, err)
	}
}

// D2-45：不同工号计数隔离（锁定不殃及他人）
func TestLoginLock_IsolationByEmployeeNo(t *testing.T) {
	s, _ := newTestScripts(t)
	ctx := context.Background()

	for i := 0; i < loginLockMaxFail+1; i++ {
		_, _ = s.LoginLockIncr(ctx, "E900004")
	}
	if is, _ := s.LoginLockIsBlocked(ctx, "E900005"); is {
		t.Fatal("不同工号不应被连带锁定")
	}
}

package service

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	goredis "github.com/redis/go-redis/v9"
)

// D2-45/D2-05：revokeUserSessions 语义单测（miniredis）——
// disabled 键写入（TTL=AT 有效期）+ 全量 RT 删除 + 重试成功路径
func TestRevokeUserSessions(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	// 两台设备的 RT
	for _, dev := range []string{"dev1", "dev2"} {
		if err := rdb.Set(ctx, refreshKey(42, dev), "hash", 168*time.Hour).Err(); err != nil {
			t.Fatal(err)
		}
	}

	if err := revokeUserSessions(ctx, rdb, 42, 30*time.Minute); err != nil {
		t.Fatal(err)
	}

	// disabled 键存在且 TTL ≈ AT 有效期
	ttl := mr.TTL("user:disabled:42")
	if ttl <= 0 || ttl > 30*time.Minute {
		t.Fatalf("disabled 键 TTL 异常: %v", ttl)
	}
	// 全部 RT 已删
	for _, dev := range []string{"dev1", "dev2"} {
		if mr.Exists(refreshKey(42, dev)) {
			t.Fatalf("RT %s 未删除", dev)
		}
	}
}

// D2-05：revokeUserSessionsWithRetry——瞬时故障后第 2 次尝试成功（退避吸收闪断）
func TestRevokeUserSessionsWithRetry_RecoversAfterTransientFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	// 第一次调用时制造一次 SET 失败（挂起后立即恢复）
	mr.SetError("boom")
	go func() {
		time.Sleep(50 * time.Millisecond)
		mr.SetError("") // 恢复
	}()

	start := time.Now()
	if err := revokeUserSessionsWithRetry(ctx, rdb, 7, 30*time.Minute); err != nil {
		t.Fatalf("瞬时故障应被重试吸收: %v", err)
	}
	if !mr.Exists("user:disabled:7") {
		t.Fatal("重试成功后 disabled 键应存在")
	}
	if elapsed := time.Since(start); elapsed < 90*time.Millisecond {
		t.Fatalf("应含退避等待（100ms），实际 %v", elapsed)
	}
}

// D2-05：持续故障——3 次尝试后返回错误（调用方感知 503，运维靠 reconcile 日志对账）
func TestRevokeUserSessionsWithRetry_PersistentFailure(t *testing.T) {
	mr := miniredis.RunT(t)
	rdb := goredis.NewClient(&goredis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	ctx := context.Background()

	mr.SetError("down")
	defer mr.SetError("")

	if err := revokeUserSessionsWithRetry(ctx, rdb, 9, 30*time.Minute); err == nil {
		t.Fatal("持续故障应返回错误")
	}
}

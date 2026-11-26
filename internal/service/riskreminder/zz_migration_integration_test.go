//go:build integration

// 迁移 000091 up/down 成对专项（文件名 zz_ 前缀——包内最后执行，down 会清表）：
// 数据在场时 down 拒绝（回滚耦合守卫），清场后 down/up 往返恢复。
package riskreminder

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestZZMigrationUpDownRefusal(t *testing.T) {
	ctx := context.Background()
	down := func() error {
		return execMigrationFile(ctx, testPool, "000091_riskreminder_accept_timeout.down.sql")
	}
	up := func() error {
		return execMigrationFile(ctx, testPool, "000091_riskreminder_accept_timeout.up.sql")
	}
	tableGone := func(t *testing.T) {
		t.Helper()
		var gone bool
		mustScan(t, testPool.QueryRow(ctx,
			`SELECT to_regclass('riskreminder_assignments') IS NULL AND to_regclass('riskreminder_intents') IS NULL`), &gone)
		if !gone {
			t.Fatal("reminder tables should be gone after down")
		}
	}

	f := createFixture(t)
	recordAssignment(t, f, 1, f.P, tDue)
	// 分派事实在场 → down 拒绝（000087 同款回滚耦合守卫）
	if err := down(); err == nil || !strings.Contains(err.Error(), "riskreminder facts exist") {
		t.Fatalf("down with facts should be refused, got %v", err)
	}

	// 清场 → down 成功、表消失
	if _, err := testPool.Exec(ctx,
		`TRUNCATE riskreminder_intents, riskreminder_assignments RESTART IDENTITY`); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if err := down(); err != nil {
		t.Fatalf("down after clear: %v", err)
	}
	tableGone(t)

	// 重新 up → 表恢复可用（后续无测试，最终态=已应用）
	if err := up(); err != nil {
		t.Fatalf("re-up: %v", err)
	}
	var ok bool
	mustScan(t, testPool.QueryRow(ctx,
		`SELECT to_regclass('riskreminder_assignments') IS NOT NULL AND to_regclass('riskreminder_intents') IS NOT NULL`), &ok)
	if !ok {
		t.Fatal("reminder tables should exist after re-up")
	}
	recordAssignment(t, f, 1, f.P, time.Now().UTC().Add(time.Minute))
}

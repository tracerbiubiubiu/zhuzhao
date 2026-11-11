package service

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
)

// out_dir 相对值须在构造时钉死为绝对（2026-11-02 taskrunner 孤儿库同类隐患治理）——
// 归档 JSONL 含审计个人信息，随启动目录漂移会落错位置且磁盘清理不接管错位产物。
func TestNewAuditArchiveJob_RelativeOutDirPinned(t *testing.T) {
	t.Run("显式相对值钉为绝对", func(t *testing.T) {
		job := NewAuditArchiveJob(nil, 0, 0, "data/archive", 0, slog.Default())
		if !filepath.IsAbs(job.outDir) {
			t.Fatalf("outDir 未钉为绝对路径: %q", job.outDir)
		}
		if want := filepath.Join("data", "archive"); !strings.HasSuffix(job.outDir, want) {
			t.Fatalf("outDir = %q, 期望以 %q 结尾", job.outDir, want)
		}
	})
	t.Run("空值走默认同样钉为绝对", func(t *testing.T) {
		job := NewAuditArchiveJob(nil, 0, 0, "", 0, slog.Default())
		if !filepath.IsAbs(job.outDir) || !strings.HasSuffix(job.outDir, filepath.Join("data", "archive")) {
			t.Fatalf("默认 outDir = %q, 期望绝对路径且以 data/archive 结尾", job.outDir)
		}
	})
	t.Run("绝对值原样保留", func(t *testing.T) {
		abs := t.TempDir()
		job := NewAuditArchiveJob(nil, 0, 0, abs, 0, slog.Default())
		if job.outDir != abs {
			t.Fatalf("outDir = %q, 期望原样 %q", job.outDir, abs)
		}
	})
}

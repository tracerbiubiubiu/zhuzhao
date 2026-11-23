// 纯函数单元测试（无 PG；进 make lint/test-unit 面）。
package riskreminder

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

func TestTruncateRunes(t *testing.T) {
	cases := []struct {
		name  string
		in    string
		max   int
		asIs  bool // 输入不超预算时原样返回
		limit int  // 结果字节上限（含省略号）
	}{
		{"short", "eligibility:withdrawn", 128, true, 128},
		{"exact", strings.Repeat("a", 512), 512, true, 512},
		{"overflow", strings.Repeat("ab", 300), 512, false, 515},
		{"multibyte", strings.Repeat("汉", 100), 128, false, 128},
	}
	for _, c := range cases {
		got := truncateRunes(c.in, c.max)
		if len(got) > c.limit {
			t.Fatalf("%s: len=%d exceeds %d", c.name, len(got), c.limit)
		}
		if c.asIs && got != c.in {
			t.Fatalf("%s: short input mutated: %q", c.name, got)
		}
		// 结果必须是合法 UTF-8（PG TEXT 拒绝坏字节序列）
		if !utf8.ValidString(got) {
			t.Fatalf("%s: invalid utf8: %q", c.name, got)
		}
	}
	if got := truncateRunes(strings.Repeat("汉", 100), 128); !strings.HasSuffix(got, "…") {
		t.Fatalf("truncated value should carry ellipsis suffix: %q", got)
	}
}

func TestDisplayTicketKey(t *testing.T) {
	if got := displayTicketKey(7, strPtr("TK-1")); got != "TK-1" {
		t.Fatalf("human key preferred, got %q", got)
	}
	if got := displayTicketKey(7, nil); got != "#7" {
		t.Fatalf("fallback id, got %q", got)
	}
	if got := displayTicketKey(7, strPtr("")); got != "#7" {
		t.Fatalf("empty human key falls back, got %q", got)
	}
}

func TestCompareAssignment(t *testing.T) {
	base := &AssignmentInput{
		TicketID: 1, Generation: 1, AssigneeID: 2, DispatchedBy: 3,
		AcceptDueAt: time.Date(2026, 6, 1, 9, 35, 0, 0, time.UTC),
		RuleID:      "r", RuleVersion: 1,
	}
	row := &Assignment{
		AssigneeID: 2, AcceptDueAt: base.AcceptDueAt, RuleID: "r", RuleVersion: 1,
	}
	if err := compareAssignment(row, base); err != nil {
		t.Fatalf("same facts should pass, got %v", err)
	}
	drift := func(mutate func(*AssignmentInput)) error {
		in := *base
		mutate(&in)
		return compareAssignment(row, &in)
	}
	if err := drift(func(in *AssignmentInput) { in.AssigneeID = 9 }); !errors.Is(err, errcode.ErrConflict) {
		t.Fatalf("assignee drift should conflict, got %v", err)
	}
	if err := drift(func(in *AssignmentInput) { in.AcceptDueAt = in.AcceptDueAt.Add(time.Minute) }); !errors.Is(err, errcode.ErrConflict) {
		t.Fatalf("due drift should conflict, got %v", err)
	}
	if err := drift(func(in *AssignmentInput) { in.RuleVersion = 2 }); !errors.Is(err, errcode.ErrConflict) {
		t.Fatalf("rule version drift should conflict, got %v", err)
	}
	if err := drift(func(in *AssignmentInput) { in.RuleID = "other" }); !errors.Is(err, errcode.ErrConflict) {
		t.Fatalf("rule id drift should conflict, got %v", err)
	}
	// dispatched_at 属记录时刻：不参与幂等比较
	if err := drift(func(in *AssignmentInput) { in.DispatchedAt = in.DispatchedAt.Add(time.Hour) }); err != nil {
		t.Fatalf("dispatched-at difference must not conflict, got %v", err)
	}
}

func TestNormalizeAssignmentInputPrecision(t *testing.T) {
	// 纳秒分量在 PG 落库时被截到微秒——规范化必须让「入库值」与「重试比较值」
	// 同精度，否则同事实重试会误判漂移冲突
	base := &AssignmentInput{
		TicketID: 1, Generation: 1, AssigneeID: 2, DispatchedBy: 3,
		DispatchedAt: time.Date(2026, 6, 1, 9, 5, 0, 123456789, time.UTC),
		AcceptDueAt:  time.Date(2026, 6, 1, 9, 35, 0, 567, time.UTC),
		RuleID:       "r", RuleVersion: 1,
	}
	n1 := normalizeAssignmentInput(base)
	n2 := normalizeAssignmentInput(base)
	if !n1.AcceptDueAt.Equal(n2.AcceptDueAt) || !n1.DispatchedAt.Equal(n2.DispatchedAt) {
		t.Fatalf("normalization not deterministic: %+v vs %+v", n1, n2)
	}
	if got := n1.AcceptDueAt; !got.Equal(base.AcceptDueAt.Truncate(time.Microsecond)) || got == base.AcceptDueAt {
		t.Fatalf("AcceptDueAt not converged to micros: %v (raw %v)", got, base.AcceptDueAt)
	}
	// µs 精度的已存行 vs 规范化后的重试 → 幂等通过；未经规范化的原始输入 → 冲突
	stored := &Assignment{AssigneeID: 2, AcceptDueAt: base.AcceptDueAt.Truncate(time.Microsecond), RuleID: "r", RuleVersion: 1}
	if err := compareAssignment(stored, n1); err != nil {
		t.Fatalf("normalized retry should be idempotent, got %v", err)
	}
	if err := compareAssignment(stored, base); !errors.Is(err, errcode.ErrConflict) {
		t.Fatalf("raw nano input against stored row should conflict (documents the fixed drift), got %v", err)
	}
	// 零值分派时钟落到当前时刻且收敛到微秒
	zero := normalizeAssignmentInput(&AssignmentInput{TicketID: 1, Generation: 1, AssigneeID: 2, DispatchedBy: 3, AcceptDueAt: base.AcceptDueAt, RuleID: "r", RuleVersion: 1})
	if zero.DispatchedAt.IsZero() || !zero.DispatchedAt.Equal(zero.DispatchedAt.Truncate(time.Microsecond)) {
		t.Fatalf("zero dispatched_at should fall to now at micro precision: %v", zero.DispatchedAt)
	}
}

func strPtr(s string) *string { return &s }

// 审计查询构造单测：method 多选（2026-10-28 审计走查批——逗号分隔 → ANY，单值向后兼容）。
package repository

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestAuditMethodMultiFilter(t *testing.T) {
	sql, args := buildAuditListWhere(AuditListQuery{Method: "GET,POST"})
	require.Contains(t, sql, "method = ANY(")
	require.Equal(t, []any{[]string{"GET", "POST"}}, args)

	sql1, args1 := buildAuditListWhere(AuditListQuery{Method: "GET"})
	require.Contains(t, sql1, "method = ANY(")
	require.Equal(t, []any{[]string{"GET"}}, args1)
}

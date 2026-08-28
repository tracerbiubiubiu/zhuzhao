package ticket

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ScopeResolver L2 可见性解析器（09-ticket §5.2）。
// 2b-core：策略 B 实体透明读——ReadAnchorPaths 返回用户可见的实体锚点路径；
// 2b-org（000012）扩展 ticket_scope group/all 路径并集与虚拟组语义。
type ScopeResolver interface {
	// ReadAnchorPaths 实体透明读锚点集合（去重）。
	// 对用户每个 user_orgs 成员，取最近实体祖先（org_type IN 1,2,3）的 path，
	// 仅当其 ticket_visibility=entity_transparent_read（09 §5.2.1）。
	ReadAnchorPaths(ctx context.Context, userID int64) ([]string, error)
}

// pgxScopeResolver 基于工单池的解析器实现（单 SQL，RK-7 量级边界内）
type pgxScopeResolver struct {
	db *pgxpool.Pool
}

// NewPgxScopeResolver 创建解析器
func NewPgxScopeResolver(db *pgxpool.Pool) ScopeResolver {
	return &pgxScopeResolver{db: db}
}

// ReadAnchorPaths 计算实体透明读锚点。
// 单 SQL 完成：user_orgs 成员 → 最近实体祖先（LATERAL 取 nlevel 最大者）→
// 过滤 ticket_visibility=entity_transparent_read。
// 2b-org 待补（000012 加列后）：expires_at 未过期过滤；source IN ('hr','system','local')
// 谓词（09 §5.2 EntityAnchorPath）；scopePathsForMembership（group/all 路径并集）。
func (r *pgxScopeResolver) ReadAnchorPaths(ctx context.Context, userID int64) ([]string, error) {
	const q = `
	SELECT DISTINCT o_anchor.path::text
	FROM user_orgs m
	JOIN organizations o_mem ON o_mem.id = m.org_id
	JOIN LATERAL (
		SELECT a.path, a.ticket_visibility
		FROM organizations a
		WHERE a.path @> o_mem.path AND a.org_type IN (1, 2, 3)
		ORDER BY nlevel(a.path) DESC
		LIMIT 1
	) o_anchor ON true
	WHERE m.user_id = $1 AND o_anchor.ticket_visibility = 'entity_transparent_read'`
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	paths, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	return paths, nil
}

// --- 角色辅助 ---

// HasRole 检查角色列表中是否含目标角色（admin/superadmin bypass 用）
func HasRole(roles []string, target string) bool {
	for _, r := range roles {
		if r == target || r == "role::"+target {
			return true
		}
	}
	return false
}

package ticket

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// 工单 scope 枚举（user_orgs.ticket_scope，000012；09-ticket §5.2 Effective scope）
const (
	ScopeAssigned = "assigned" // 仅属主（创建人/处理人）
	ScopeGroup    = "group"    // 成员组织子树（主管）
	ScopeAll      = "all"      // 全量（仍受路由级 Casbin 约束）
)

// ResolvedScope 用户工单可见性解析结果（09-ticket §5.2）。
// 三轴正交：透明读锚点（策略 B）/ scope 扩展路径 / 全量开关，OR 合并、不互相替代。
type ResolvedScope struct {
	AnchorPaths []string // 策略 B 实体透明读锚点（最近实体祖先，受 ticket_visibility 门控）
	ScopePaths  []string // ticket_scope=group 的成员组织子树路径（主管扩展，不受 visibility 门控）
	AllScope    bool     // 任一成员 ticket_scope=all → 列表全量、全部工单可见
}

// ReadPaths L2 可见路径集 = 透明锚点 ∪ group 作用域子树
func (s *ResolvedScope) ReadPaths() []string {
	paths := make([]string, 0, len(s.AnchorPaths)+len(s.ScopePaths))
	paths = append(paths, s.AnchorPaths...)
	paths = append(paths, s.ScopePaths...)
	return paths
}

// ScopeResolver L2 可见性解析器。
// 2b-core 引入实体透明读；2b-org（000012）启用 ticket_scope / expires_at / source 列。
type ScopeResolver interface {
	// ReadAnchorPaths L2 可见路径集（透明锚点 ∪ group 作用域子树；PRD §5.2 命名）
	ReadAnchorPaths(ctx context.Context, userID int64) ([]string, error)
	// ResolveScope 完整解析结果（assign 主管判定 / 全量开关消费）
	ResolveScope(ctx context.Context, userID int64) (*ResolvedScope, error)
}

// pgxScopeResolver 基于工单池的解析器实现（单 SQL，RK-7 量级边界内）
type pgxScopeResolver struct {
	db *pgxpool.Pool
}

// NewPgxScopeResolver 创建解析器
func NewPgxScopeResolver(db *pgxpool.Pool) ScopeResolver {
	return &pgxScopeResolver{db: db}
}

// resolve 单 SQL 完成三轴归并：
//   - 成员资格：expires_at 未过期（临时成员过期即失效，读取侧过滤——03-org-enhance）
//   - 透明锚点：最近实体祖先（NOT is_virtual）且 ticket_visibility=entity_transparent_read；
//     虚拟组成员的锚点为挂载实体而非虚拟组自身（09 §5.2 EntityAnchorPath）
//   - scope：group → 成员组织子树路径并入；all → AllScope；assigned → 不扩展（仅属主）
//
// source 谓词说明（09 §5.2 EntityAnchorPath 的 source IN ('hr','system','local')）：
// 000012 起三值皆可为实体来源（HR 延后期间实体为 local），无需过滤；HR Sync（2b-ext）
// 只写 source='hr'，软删对账不触碰 local。
func (r *pgxScopeResolver) resolve(ctx context.Context, userID int64) (*ResolvedScope, error) {
	const q = `
	SELECT o_anchor.path::text, o_anchor.ticket_visibility, m_org.path::text, m.ticket_scope
	FROM user_orgs m
	JOIN organizations m_org ON m_org.id = m.org_id
	JOIN LATERAL (
		SELECT a.path, a.ticket_visibility
		FROM organizations a
		WHERE a.path @> m_org.path AND NOT a.is_virtual
		ORDER BY nlevel(a.path) DESC
		LIMIT 1
	) o_anchor ON true
	WHERE m.user_id = $1
	  AND (m.expires_at IS NULL OR m.expires_at > NOW())`
	rows, err := r.db.Query(ctx, q, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	scope := &ResolvedScope{AnchorPaths: []string{}, ScopePaths: []string{}}
	seenAnchor := map[string]bool{}
	seenScope := map[string]bool{}
	for rows.Next() {
		var anchorPath, anchorVisibility, memberPath, ticketScope string
		if err := rows.Scan(&anchorPath, &anchorVisibility, &memberPath, &ticketScope); err != nil {
			return nil, err
		}
		switch ticketScope {
		case ScopeAll:
			scope.AllScope = true
		case ScopeGroup:
			if !seenScope[memberPath] {
				seenScope[memberPath] = true
				scope.ScopePaths = append(scope.ScopePaths, memberPath)
			}
		default: // assigned 或脏值：不扩展（仅属主）
		}
		if anchorVisibility == "entity_transparent_read" && !seenAnchor[anchorPath] {
			seenAnchor[anchorPath] = true
			scope.AnchorPaths = append(scope.AnchorPaths, anchorPath)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return scope, nil
}

func (r *pgxScopeResolver) ResolveScope(ctx context.Context, userID int64) (*ResolvedScope, error) {
	return r.resolve(ctx, userID)
}

func (r *pgxScopeResolver) ReadAnchorPaths(ctx context.Context, userID int64) ([]string, error) {
	scope, err := r.resolve(ctx, userID)
	if err != nil {
		return nil, err
	}
	return scope.ReadPaths(), nil
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

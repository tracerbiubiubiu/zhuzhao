package ticket

import (
	"context"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

// TicketScope 工单可见性范围
type TicketScope string

const (
	ScopeAssigned TicketScope = "assigned" // 2a：仅本人创建/被分派
	ScopeGroup    TicketScope = "group"    // 2b：组内
	ScopeAll      TicketScope = "all"      // 2b：全量
)

// ScopeResolver 解析用户对组织的 effective ticket_scope。
// 2a 桩实现：EffectiveTicketScope 恒返回 assigned，其余返回 nil。
// 2b 将由 authz.ScopeResolver 替换（ReadAnchorPaths 等实体透明读逻辑）。
type ScopeResolver interface {
	// EffectiveTicketScope 用户在某 org 的 ticket_scope（2a 固定 assigned）
	EffectiveTicketScope(ctx context.Context, userID, orgID int64) (TicketScope, error)
	// VisibleOrgPaths 2b 实现；2a 返回 nil
	VisibleOrgPaths(ctx context.Context, userID int64) ([]string, error)
	// ReadAnchorPaths 2b 策略 B 实体透明读锚点 + scope group/all 路径并集
	ReadAnchorPaths(ctx context.Context, userID int64) ([]string, error)
}

// stubScopeResolver 2a 桩实现
type stubScopeResolver struct{}

// NewStubScopeResolver 创建 2a ScopeResolver 桩
func NewStubScopeResolver() ScopeResolver {
	return &stubScopeResolver{}
}

func (s *stubScopeResolver) EffectiveTicketScope(_ context.Context, _, _ int64) (TicketScope, error) {
	return ScopeAssigned, nil
}

func (s *stubScopeResolver) VisibleOrgPaths(_ context.Context, _ int64) ([]string, error) {
	return nil, nil
}

func (s *stubScopeResolver) ReadAnchorPaths(_ context.Context, _ int64) ([]string, error) {
	return nil, nil
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

// model.Organization 占位引用（2b ScopeResolver 实现需要）
var _ = model.Organization{}

// S10 B10-3 待办角标聚合（10 号 §3.3 + 附录 A.4）：sections 开放 map，
// 单请求多计数（替代首页每状态一次 total 的权宜）。
//
// 计算器注册制（A.1）：各域向本枢纽注册 section 计算函数，handler 只做编排——
// phase6 risk 域注册 "risk" 时 import 本宿主即可（域边界：禁止往 ticket 域
// service 里塞 risk 计数；定位注 2026-10-16 外审——跨域枢纽宿主 ticket 域可接受，
// phase6 注册时再议升位）。未读数（unread）S6 二期并入同端点 sections。
package ticket

import (
	"context"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"
)

// SectionFunc 单 section 计数函数（域自注册）。
type SectionFunc func(ctx context.Context, userID int64) (int64, error)

// BadgeService 角标聚合服务。
type BadgeService struct {
	db       *pgxpool.Pool
	sections map[string]SectionFunc
	logger   *slog.Logger
}

// NewBadgeService 创建并完成 ticket 域自注册（todo）。
// unread section 经 RegisterUnread 由装配层注入（B6-7 inbox 依赖——避免
// badge 直接依赖 inbox service 的构造环）。
func NewBadgeService(db *pgxpool.Pool) *BadgeService {
	s := &BadgeService{db: db, sections: map[string]SectionFunc{}, logger: slog.Default()}
	s.Register("todo", s.todoCount)
	return s
}

// RegisterUnread 注册未读数 section（S6 B6-7——inbox service 装配后调用）。
func (s *BadgeService) RegisterUnread(fn SectionFunc) {
	s.Register("unread", fn)
}

// Register 注册/覆盖一个 section 计算器（域内装配期调用；非并发安全——仅启动期）。
func (s *BadgeService) Register(name string, fn SectionFunc) {
	s.sections[name] = fn
}

// Sections 逐 section 计算。角标是提示面：单 section 失败不整体失败——
// 该 section 记日志跳过（前端对缺失 section 容错忽略——A.4 向前兼容契约）。
func (s *BadgeService) Sections(ctx context.Context, userID int64) map[string]int64 {
	out := make(map[string]int64, len(s.sections))
	for name, fn := range s.sections {
		n, err := fn(ctx, userID)
		if err != nil {
			s.logger.Warn("badge: section failed", "section", name, "user", userID, "err", err)
			continue
		}
		out[name] = n
	}
	return out
}

// todoCount 待办工单数（assignee=me 且非终态）。A.4 口径：不走 GetFilter 全量
// ——assignee 谓词即本人可见性（分派只落在可见集内），直查 O(索引)。
func (s *BadgeService) todoCount(ctx context.Context, userID int64) (int64, error) {
	var n int64
	err := s.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM tickets WHERE assigned_to = $1 AND status NOT IN ('closed', 'rejected')`,
		userID).Scan(&n)
	return n, err
}

// B2 第 7 项「个人派单与部门统计」读取/聚合接缝（kickoff 计划第 7 项 + §1 口径）。
//
// 口径契约：
//   - 当前待办 = 在办派单组（risk_dispatch_groups.ended_at IS NULL），归属 = 组责任人
//     （owner_user_id，派单合同的责任主体；工单 assigned_to 属工单域状态，本接缝不混用），
//     部门 = 该责任人**现势主部门**（user_orgs.is_primary 查询时点读取）；
//   - 历史统计 = 派单成功记录全量，归属 = 组行冻结的 owner_user_id + owner_org_at_dispatch，
//     不随后续调部门/主体软删改写；
//   - 权限检查先于计数 = 先解析当前主体工单可见性（TicketVisibilityFilter，复用首页/
//     工单列表 GetFilter 同源能力），解析失败或未显式豁免的空谓词一律 fail-closed 拒绝
//     计数（沿 resource.Filter IW4 哨兵纪律）；可见性谓词压在聚合前的行级过滤上，
//     不可见票不进任何计数；
//   - 部门仅归属展示 = 部门列只做统计分桶与展示，本接缝不产出任何按部门放行的授权
//     事实——任何组织成员都不因部门获得风险或工单（可见性只走工单域谓词）；
//   - 工单目标组织不随迁 = tickets.org_id 恒为派单合同目标组织，责任人调部门不改写
//     工单归属，本接缝只读不改。
//
// 在职/删除约定（沿项目真实字段）：现势主部门 = users.deleted_at IS NULL 的主体的
// user_orgs 主部门行；主体软删后归 0 桶（未归属），停用（status<>1）不动组织归属、
// 在办票不消失；owner_org_at_dispatch 为 0/NULL（派单时无主部门）同样归 0 桶。
// 端点级 L1（菜单 GET + Casbin）由组合根挂接，本接缝只承担行级可见性与聚合。
package riskdispatch

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

// TicketVisibilityFilter 工单当前可见性谓词端口：复用首页/工单列表当前可见票过滤
// （ticket 资源 GetFilter 同源）。组合根用 GetFilter 薄适配（测试含参考形状），
// 本包不 import ticket——沿 TicketCreator 端口同款域边界。
//
// 合同：返回以**未别名 tickets 表**为行源的布尔谓词，且只允许 $1（主体 ID）与
// $2（ltree 路径数组）两个参数位（GetFilter 现行形状）；unscoped=true 为显式豁免
// （scope=all / admin bypass），此时谓词与参数均不消费。适配器不得改写参数序。
type TicketVisibilityFilter interface {
	VisibleTicketsWhere(ctx context.Context, actorID int64) (where string, args []any, unscoped bool, err error)
}

// DeptTodoRow 当前待办部门汇总行（展示归属；计数已过当前权限过滤）。
type DeptTodoRow struct {
	DeptID      int64  `json:"dept_id,string"` // 责任人现主部门（0 = 未归属：无主部门/主体已软删）
	DeptName    string `json:"dept_name"`
	OpenTickets int64  `json:"open_tickets"` // 在办工单数（同票计一次）
	OpenRisks   int64  `json:"open_risks"`   // 活跃关联风险数（资产/项口径）
}

// DeptHistoryRow 历史派单部门汇总行（按派单成功时冻结的责任人 + 部门）。
type DeptHistoryRow struct {
	OwnerUserID int64  `json:"owner_user_id,string"`
	OwnerName   string `json:"owner_name"`     // 软删主体为空串（冻结行不消失）
	DeptID      int64  `json:"dept_id,string"` // owner_org_at_dispatch（0 = 派单时未归属）
	DeptName    string `json:"dept_name"`
	Dispatches  int64  `json:"dispatches"` // 派单成功组数
	Tickets     int64  `json:"tickets"`    // 涉及工单数（组票 1:1，同票单口径计一次）
	Risks       int64  `json:"risks"`      // 涉及风险关联数（重提重链不双计）
}

type DepartmentStatsResult struct {
	Current []DeptTodoRow    `json:"current"`
	History []DeptHistoryRow `json:"history"`
}

// DeptStats 部门读取/聚合接缝（只读投影，不产生任何授权事实）。
type DeptStats struct {
	visibility TicketVisibilityFilter
}

// NewDeptStats 装配：注入工单可见性谓词端口（组合根接 GetFilter 适配器）。
func NewDeptStats(visibility TicketVisibilityFilter) *DeptStats {
	return &DeptStats{visibility: visibility}
}

// CurrentTodoByDept 当前待办按责任人现主部门汇总。
// 权限先于计数：先解析可见性（失败即返回错误、零计数），再对行级过滤后的在办组聚合。
func (s *DeptStats) CurrentTodoByDept(ctx context.Context, db Reader, actorID int64) ([]DeptTodoRow, error) {
	sql, args, err := s.assemble(ctx, actorID, deptTodoSQLTemplate)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query dept todo stats: %w", err)
	}
	defer rows.Close()
	out := []DeptTodoRow{}
	for rows.Next() {
		var row DeptTodoRow
		if err := rows.Scan(&row.DeptID, &row.DeptName, &row.OpenTickets, &row.OpenRisks); err != nil {
			return nil, fmt.Errorf("scan dept todo stats: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dept todo stats: %w", err)
	}
	return out, nil
}

// HistoryByDispatchDept 历史派单按派单成功时冻结的责任人与部门汇总。
// 同一可见性合同：不可见票的派单记录不进当前查询者的计数。
func (s *DeptStats) HistoryByDispatchDept(ctx context.Context, db Reader, actorID int64) ([]DeptHistoryRow, error) {
	sql, args, err := s.assemble(ctx, actorID, deptHistorySQLTemplate)
	if err != nil {
		return nil, err
	}
	rows, err := db.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("query dept history stats: %w", err)
	}
	defer rows.Close()
	out := []DeptHistoryRow{}
	for rows.Next() {
		var row DeptHistoryRow
		if err := rows.Scan(&row.OwnerUserID, &row.OwnerName, &row.DeptID, &row.DeptName,
			&row.Dispatches, &row.Tickets, &row.Risks); err != nil {
			return nil, fmt.Errorf("scan dept history stats: %w", err)
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate dept history stats: %w", err)
	}
	return out, nil
}

// deptVisibilitySlot 模板中可见性谓词的落位标记。
const deptVisibilitySlot = "/*visibility*/"

// assemble 先解析可见性（先于计数），再把谓词压进模板行级过滤位。
func (s *DeptStats) assemble(ctx context.Context, actorID int64, template string) (string, []any, error) {
	if s == nil || s.visibility == nil || actorID <= 0 {
		return "", nil, errcode.ErrNoPermission
	}
	where, args, unscoped, err := s.visibility.VisibleTicketsWhere(ctx, actorID)
	if err != nil {
		return "", nil, fmt.Errorf("resolve ticket visibility: %w", err)
	}
	slot := ""
	if unscoped {
		// 显式豁免（scope=all / admin bypass）：不再行级过滤；参数随谓词一并弃用。
		args = nil
	} else {
		if strings.TrimSpace(where) == "" {
			// 沿 resource.Filter 哨兵纪律（IW4）：未显式豁免的空谓词不是合法全量，
			// fail-closed 拒绝计数，绝不静默放开。
			return "", nil, fmt.Errorf("empty ticket visibility predicate: %w", errcode.ErrForbidden)
		}
		slot = " AND (" + where + ")"
	}
	actorParam := "$" + strconv.Itoa(len(args)+1)
	args = append(args, actorID)
	links := "WITH visible_links AS (" + repository.RiskDispatchVisibleLinksSQL(actorParam) + ") "
	return links + strings.Replace(template, deptVisibilitySlot, slot, 1), args, nil
}

// deptTodoSQLTemplate 当前待办聚合：在办组 × 可见票 × 现势主部门。
// 可见性片段引用 tickets 未别名列与 $1/$2——本模板不得引入其他参数位。
const deptTodoSQLTemplate = `
SELECT COALESCE(cur.org_id, 0)     AS dept_id,
       COALESCE(cur.org_name, '')  AS dept_name,
       COUNT(DISTINCT g.ticket_id) AS open_tickets,
       COUNT(DISTINCT l.risk_id)   AS open_risks
FROM risk_dispatch_groups g
JOIN tickets ON tickets.id = g.ticket_id /*visibility*/
JOIN visible_links l
       ON l.group_id = g.id AND l.ended_at IS NULL
LEFT JOIN LATERAL (
       SELECT uo.org_id, o.name AS org_name
       FROM user_orgs uo
       JOIN users u ON u.id = uo.user_id AND u.deleted_at IS NULL
       JOIN organizations o ON o.id = uo.org_id
       WHERE uo.user_id = g.owner_user_id AND uo.is_primary
       LIMIT 1
     ) cur ON true
WHERE g.ended_at IS NULL
GROUP BY cur.org_id, cur.org_name
ORDER BY 1`

// deptHistorySQLTemplate 历史聚合：派单记录 × 可见票 × 派单时点冻结责任人/部门。
const deptHistorySQLTemplate = `
SELECT g.owner_user_id,
       COALESCE(NULLIF(u.real_name, ''), u.username, '') AS owner_name,
       COALESCE(g.owner_org_at_dispatch, 0)              AS dept_id,
       COALESCE(o.name, '')                              AS dept_name,
       COUNT(DISTINCT g.id)                              AS dispatches,
       COUNT(DISTINCT g.ticket_id)                       AS tickets,
       COUNT(DISTINCT l.risk_id)                         AS risks
FROM risk_dispatch_groups g
JOIN tickets ON tickets.id = g.ticket_id /*visibility*/
JOIN visible_links l ON l.group_id = g.id
LEFT JOIN users u ON u.id = g.owner_user_id AND u.deleted_at IS NULL
LEFT JOIN organizations o ON o.id = g.owner_org_at_dispatch
GROUP BY g.owner_user_id, u.real_name, u.username, g.owner_org_at_dispatch, o.name
ORDER BY 3, 1`

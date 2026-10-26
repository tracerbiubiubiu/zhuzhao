// S10 B10-5 工单导出 handler（10 号 §3.5/附录 A.6）：excelize StreamWriter
// 流式（逐行 flush 防大结果集驻留）；与 List 同 parseListQuery + 同
// resolveListFilter/buildTicketWhere（BK-21 代码级同源——导出不越查询可见性）；
// 行数钳制 5 万；导出自审计（谁/筛选/行数——A.6 业界标配，流式响应不经通用
// 审计中间件的 body 捕获，挂独立链 + handler 手动落行）。
package handler

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/xuri/excelize/v2"

	"github.com/tracerbiubiubiu/zhuzhao-utils/response"
	"github.com/tracerbiubiubiu/zhuzhao/internal/middleware"
	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	ticketsvc "github.com/tracerbiubiubiu/zhuzhao/internal/service/ticket"
)

// exportTicketColumns 导出固定列（A.6——列头与列序为本契约的一部分）
var exportTicketColumns = []string{
	"ID", "编号", "类型", "标题", "描述", "优先级", "状态", "创建人ID", "创建人", "处理人ID", "处理人", "组织路径", "创建时间", "更新时间", "进入状态时间",
}

func fmtTime(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// Export GET /api/v1/tickets/export
//
//	@Summary	工单导出（xlsx 流式——与列表同筛选/同可见性；行数上限 5 万）
//	@Tags		ticket
//	@Produce	application/octet-stream
//	@Param		type_code		query	string	false	"工单类型"
//	@Param		status			query	string	false	"工单状态"
//	@Param		priority		query	int		false	"优先级"
//	@Param		tag				query	string	false	"标签（S10 B10-2）"
//	@Param		assignee		query	string	false	"处理人过滤，仅支持 me"
//	@Param		created_by		query	string	false	"创建人过滤，仅支持 me"
//	@Param		keyword			query	string	false	"标题关键字"
//	@Param		created_from	query	string	false	"创建日期起（YYYY-MM-DD）"
//	@Param		created_to		query	string	false	"创建日期止（YYYY-MM-DD）"
//	@Success	200	{file}	file	"xlsx 附件"
//	@Router		/api/v1/tickets/export [get]
func (h *TicketHandler) Export(c *gin.Context) {
	q, ok := parseListQuery(c)
	if !ok {
		return
	}

	f := excelize.NewFile()
	sw, err := f.NewStreamWriter("Sheet1")
	if err != nil {
		response.InternalError(c, "导出初始化失败")
		return
	}
	_ = sw.SetColWidth(1, len(exportTicketColumns), 18)
	if err := sw.SetRow("A1", anyRow(exportTicketColumns)); err != nil {
		response.InternalError(c, "导出初始化失败")
		return
	}
	rowIdx := 2
	rowFn := func(t *model.Ticket) error {
		cell, cerr := excelize.CoordinatesToCellName(1, rowIdx)
		if cerr != nil {
			return cerr
		}
		assigneeID := ""
		if t.AssignedTo != nil {
			assigneeID = fmt.Sprintf("%d", *t.AssignedTo)
		}
		row := []interface{}{
			t.ID, t.HumanKey, t.TypeCode, t.Title, t.Description, t.Priority, t.Status,
			t.CreatedBy, joinNonEmpty(t.CreatedByName, t.CreatedByNo), assigneeID, joinNonEmpty(t.AssigneeName, t.AssigneeNo),
			t.OrgPath, fmtTime(&t.CreatedAt), fmtTime(&t.UpdatedAt), fmtTime(t.StatusChangedAt),
		}
		rowIdx++
		return sw.SetRow(cell, row)
	}

	total, err := h.ticketService.ExportStream(c.Request.Context(), q, c.GetInt64("userID"), rowFn)
	if err != nil {
		if errors.Is(err, ticketsvc.ErrExportTooMany) {
			response.BadRequest(c, err.Error())
			return
		}
		writeServiceError(c, err)
		return
	}

	// 导出自审计（A.6：谁/筛选/行数——流式挂独立链不经通用审计中间件，手动落行）。
	// 落在 f.Write 之前：行数此刻已知，客户端中途断开也须留痕（数据已生成即视为已导出）；
	// ctx 用 WithoutCancel——客户端断开会取消请求 ctx，审计写入不得随之丢失。
	if h.audit != nil {
		_ = h.audit.Insert(context.WithoutCancel(c.Request.Context()), middleware.AuditLogEntry{
			UserID:          c.GetInt64("userID"),
			Username:        c.GetString("username"),
			Method:          "GET",
			Path:            "/api/v1/tickets/export",
			StatusCode:      200,
			Duration:        0,
			IP:              c.ClientIP(),
			UserAgent:       c.Request.UserAgent(),
			RequestBody:     c.Request.URL.RawQuery, // 筛选条件（谁在导什么范围的行）
			ResponseSummary: fmt.Sprintf("rows=%d", total),
			RequestID:       c.GetString("request_id"), // 与 middleware/logger.go 同键（原 X-Request-ID 键无人设置，恒空）
			CreatedAt:       time.Now(),
		})
	}

	filename := "tickets_" + time.Now().UTC().Format("20060102_150405") + ".xlsx"
	c.Header("Content-Disposition",
		fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, filename, filename))
	c.Header("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	c.Status(200)
	if err := f.Write(c.Writer); err != nil {
		// 响应头已发——写失败只能截断（客户端重试即全量重导，幂等）
		_ = c.Error(err)
		return
	}
}

// anyRow []string → []interface{}（excelize SetRow 形态）
func anyRow(cols []string) []interface{} {
	row := make([]interface{}, len(cols))
	for i, c := range cols {
		row[i] = c
	}
	return row
}

// joinNonEmpty 富化名拼接（人名/工号空段跳过）
func joinNonEmpty(parts ...string) string {
	out := ""
	for _, p := range parts {
		if p == "" {
			continue
		}
		if out != "" {
			out += "/"
		}
		out += p
	}
	return out
}

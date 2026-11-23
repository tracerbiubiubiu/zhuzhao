// 扫描与消费执行面：显式 asOf 遍历生成持久待发意图；消费在现势复核后
// 「站内入箱 + 成功事实」同事务（一次性效果）。生成与消费分开——验收卡
// 「09:36 生成一条待发送的站内提醒」与正常投递是两个可独立暂停的环节。
package riskreminder

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/domainevent"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

// failedRetryBackoff 持续失败条目的健康优先窗口：窗口内刚试败的条目让位于
// 健康条目（防最老失败条目队头压制、健康条目跨轮饥饿）；窗口外或本轮无健康
// 条目时照常按原键重试——恢复语义不回退。
const failedRetryBackoff = 30 * time.Second

// GenerateDue 显式 asOf 遍历：到期未接单且仍处当前代次的分派 → 落持久 pending
// 意图（自管事务）。重复扫描经七元组投递键唯一约束去重——重启/重扫不重发同一时隙。
func (s *Service) GenerateDue(ctx context.Context, asOf time.Time, limit int) ([]*Intent, error) {
	if limit <= 0 {
		limit = DefaultScanLimit
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return nil, err
	}
	intents, err := s.GenerateDueTx(ctx, tx, asOf, limit)
	if err != nil {
		_ = tx.Rollback(ctx)
		return nil, err
	}
	return intents, tx.Commit(ctx)
}

// GenerateDueTx GenerateDue 的事务版（主会话可挂入既有调度事务；limit<=0 取默认）。
// LIMIT 只数「尚无同键意图」的分派——已键控（含已终态）的先被 NOT EXISTS 排除，
// 防最老 N 条霸占配额、更晚到期的分派永远排不进来；ON CONFLICT 兜底并发双扫。
func (s *Service) GenerateDueTx(ctx context.Context, tx pgx.Tx, asOf time.Time, limit int) ([]*Intent, error) {
	if limit <= 0 {
		limit = DefaultScanLimit
	}
	rows, err := tx.Query(ctx, `
		INSERT INTO riskreminder_intents
			(assignment_id, ticket_id, generation, recipient_id, due_at,
			 source, source_event_id, rule_id, rule_version, channel, semantic_key)
		SELECT a.id, a.ticket_id, a.generation, a.assignee_id, a.accept_due_at,
		       $1, a.id::text, a.rule_id, a.rule_version, $2, $3
		FROM riskreminder_assignments a
		WHERE a.accepted_at IS NULL
		  AND a.accept_due_at <= $4
		  AND NOT EXISTS (
		      SELECT 1 FROM riskreminder_assignments n
		      WHERE n.ticket_id = a.ticket_id AND n.generation > a.generation)
		  AND NOT EXISTS (
		      SELECT 1 FROM riskreminder_intents i
		      WHERE i.source = $1 AND i.source_event_id = a.id::text
		        AND i.rule_id = a.rule_id AND i.rule_version = a.rule_version
		        AND i.recipient_id = a.assignee_id
		        AND i.channel = $2 AND i.semantic_key = $3)
		ORDER BY a.accept_due_at, a.id
		LIMIT $5
		ON CONFLICT (source, source_event_id, rule_id, rule_version, recipient_id, channel, semantic_key)
		DO NOTHING
		RETURNING `+intentCols,
		Source, Channel, SemanticKey, asOf, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return collectIntents(rows)
}

// DeliverDue 消费一轮到期 pending 意图（每意图独立事务；SKIP LOCKED 支持多消费者）。
//
// 取锁顺序恒为 票→意图：先取真实 IAM 共享守卫（repository.WithIAMCommandGuard，先于 Begin），再取票锁；
// IAM 撤权持同键独占守卫，因此撤权提交与投递提交全序化——
// 撤权先提交则守卫内必读到撤权态，投递先持锁则撤权等提交后行；意图行锁其后
// 才取（SKIP LOCKED）。意图锁之后不再取任何 IAM/票锁。锁内复核顺序：已接单 →
// 更新代次 → 现势收件人资格（checker）；全部通过才入箱 + 终态同事务提交。
//
// 候选拾取健康优先：刚试败（backoff 窗口内）的条目让位健康条目，防队头压制；
// 同轮内已尝试的意图不再重复拾取（防毒意图打满 limit）。事务失败回滚后独立
// 留痕 attempt/last_error——状态仍 pending，可按原键恢复，下一轮照常恢复；
// 抑止与失败在报告中分别可见，空轮次正常返回零结果。
func (s *Service) DeliverDue(ctx context.Context, asOf time.Time, limit int) (*DeliveryReport, error) {
	if limit <= 0 {
		limit = DefaultDeliverLimit
	}
	report := &DeliveryReport{}
	tried := make(map[int64]struct{}, limit)
	for len(report.Delivered)+len(report.Suppressed)+len(report.Failed) < limit {
		processed, err := s.deliverOne(ctx, asOf, tried, report)
		if err != nil {
			return nil, err
		}
		if !processed {
			break
		}
	}
	return report, nil
}

// deliverOne 处理恰好一条到期意图（tried 内的本轮已尝试）；无到期 pending 时返回
// processed=false。票已删/意图竞争输家（SKIP LOCKED 落空）按已处理返回——本轮
// 换下一条，tried 表保证推进。
func (s *Service) deliverOne(ctx context.Context, asOf time.Time, tried map[int64]struct{}, report *DeliveryReport) (bool, error) {
	var intent, outcome *Intent
	processed := false
	var processErr error
	err := s.runDeliveryTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		skip := make([]int64, 0, len(tried))
		for id := range tried {
			skip = append(skip, id)
		}
		var candID, ticketID int64
		err := tx.QueryRow(ctx, `SELECT id,ticket_id FROM riskreminder_intents
   WHERE status=$1 AND due_at <= $2 AND NOT(id=ANY($3::bigint[]))
   ORDER BY (last_attempted_at IS NOT NULL AND last_attempted_at > $4),due_at,id LIMIT 1`,
			StatusPending, asOf, skip, asOf.Add(-failedRetryBackoff)).Scan(&candID, &ticketID)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		tried[candID] = struct{}{}
		processed = true
		var guard int64
		err = tx.QueryRow(ctx, `SELECT id FROM tickets WHERE id=$1 FOR UPDATE`, ticketID).Scan(&guard)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		intent, err = scanIntent(tx.QueryRow(ctx, `SELECT `+intentCols+` FROM riskreminder_intents
   WHERE id=$1 AND status=$2 FOR UPDATE SKIP LOCKED`, candID, StatusPending))
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		outcome, processErr = s.processIntent(ctx, tx, intent)
		return processErr
	})
	if err != nil {
		if intent == nil {
			return false, err
		}
		if processErr != nil {
			s.recordAttemptFailure(ctx, intent.ID, processErr)
			report.Failed = append(report.Failed, DeliveryFailure{IntentID: intent.ID, Err: processErr})
		} else {
			s.settleCommitUnknown(ctx, intent, err, report)
		}
		return true, nil
	}
	if outcome != nil {
		switch outcome.Status {
		case StatusDelivered:
			report.Delivered = append(report.Delivered, outcome)
		case StatusSuppressed:
			report.Suppressed = append(report.Suppressed, outcome)
		default:
			return false, fmt.Errorf("riskreminder: unexpected intent %d status %q", outcome.ID, outcome.Status)
		}
	}
	return processed, nil
}

// settleCommitUnknown 提交确认不明（连接断开/上下文取消等——服务器可能已提交）：
// 用独立读回（脱离调用方可能已取消的上下文）仲裁真实结果。已终态 → 按真实结果
// 入报告、不留失败证据；确认仍 pending → 按失败留证（原键可恢复）；读回也不可达
// → 失败证据标 Indeterminate——不宣称真实送达。
func (s *Service) settleCommitUnknown(ctx context.Context, i *Intent, commitErr error, report *DeliveryReport) {
	// pgx 对已取消的 ctx 拒绝一切操作——读回与留证脱离它，限时自兜底
	rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	final, rerr := readIntentRow(rctx, s.db, i.ID)
	switch {
	case rerr == nil && final.Status == StatusDelivered:
		report.Delivered = append(report.Delivered, final) // 真实送达——报告真实结果
	case rerr == nil && final.Status == StatusSuppressed:
		report.Suppressed = append(report.Suppressed, final) // 真实抑止——报告真实结果
	case rerr == nil: // 确认未提交——按失败留证，原键可恢复
		s.recordAttemptFailure(rctx, i.ID, commitErr)
		report.Failed = append(report.Failed, DeliveryFailure{IntentID: i.ID, Err: commitErr})
	default: // 连读回都不可达——真实状态未知，明示不可判定
		s.recordAttemptFailure(rctx, i.ID, commitErr)
		report.Failed = append(report.Failed, DeliveryFailure{
			IntentID:      i.ID,
			Err:           fmt.Errorf("commit outcome unknown (readback: %v): %w", rerr, commitErr),
			Indeterminate: true,
		})
	}
}

// processIntent 锁内复核 + 入箱 + 终态，全部发生在调用方事务内。
// 调用方已持 IAM 共享守卫、票行锁与意图行锁——此处对 tickets 只读、不再取锁。
func (s *Service) processIntent(ctx context.Context, tx pgx.Tx, i *Intent) (*Intent, error) {
	// 分派事实复核：已接单/更新代次 → 抑止（原因留痕）
	var a Assignment
	if err := tx.QueryRow(ctx, `SELECT `+assignmentCols+`
		FROM riskreminder_assignments WHERE id = $1`, i.AssignmentID,
	).Scan(&a.ID, &a.TicketID, &a.Generation, &a.AssigneeID, &a.DispatchedBy,
		&a.DispatchedAt, &a.AcceptDueAt, &a.RuleID, &a.RuleVersion, &a.AcceptedAt, &a.AcceptedBy,
	); err != nil {
		return nil, err
	}
	if a.AcceptedAt != nil {
		return suppressIntentTx(ctx, tx, i.ID, ReasonAccepted)
	}
	var superseded bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM riskreminder_assignments
			WHERE ticket_id = $1 AND generation > $2)`,
		a.TicketID, a.Generation,
	).Scan(&superseded); err != nil {
		return nil, err
	}
	if superseded {
		return suppressIntentTx(ctx, tx, i.ID, ReasonSuperseded)
	}

	// 现势收件人资格（主会话接线工单状态面；nil checker 视为未装配——按失败留痕，
	// 不抑止不入箱，装配后按原键恢复）
	if s.checker == nil {
		return nil, errcode.New(errcode.ErrServiceUnavailable.Code, "riskreminder: eligibility checker not wired")
	}
	eligible, reason, cerr := s.checker(ctx, tx, i.TicketID, i.RecipientID)
	if cerr != nil {
		return nil, fmt.Errorf("eligibility check: %w", cerr)
	}
	if !eligible {
		if reason == "" {
			reason = "eligibility_withdrawn"
		}
		return suppressIntentTx(ctx, tx, i.ID, truncateRunes(reason, 128))
	}

	// 站内入箱 + 成功事实同事务（一次性效果）；深链与 biz 锚点沿 inbox 通道惯例
	var humanKey *string
	if err := tx.QueryRow(ctx, `SELECT human_key FROM tickets WHERE id = $1`, i.TicketID).Scan(&humanKey); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// 常态不可达（票删经 CASCADE 连带清意图，且守卫先行）——防御留抑止原因
			return suppressIntentTx(ctx, tx, i.ID, "ticket_gone")
		}
		return nil, err
	}
	title := "工单超时未接单：" + displayTicketKey(i.TicketID, humanKey)
	row := &model.NotificationRow{
		RecipientID: i.RecipientID,
		BizType:     string(domainevent.BizTicket),
		BizID:       &i.TicketID,
		Title:       title,
		Link:        "/tickets/" + strconv.FormatInt(i.TicketID, 10),
	}
	if err := s.inbox.InsertTx(ctx, tx, row); err != nil {
		return nil, fmt.Errorf("inbox insert: %w", err)
	}
	tag, err := tx.Exec(ctx, `
		UPDATE riskreminder_intents
		SET status = $3, notification_id = $4, delivered_at = now()
		WHERE id = $1 AND status = $2`,
		i.ID, StatusPending, StatusDelivered, row.ID)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, errcode.ErrConflict
	}
	return readIntentTx(ctx, tx, i.ID)
}

// ─── 内部 ───

func suppressIntentTx(ctx context.Context, tx pgx.Tx, intentID int64, reason string) (*Intent, error) {
	tag, err := tx.Exec(ctx, `
		UPDATE riskreminder_intents
		SET status = $3, suppressed_at = now(), suppress_reason = $2
		WHERE id = $1 AND status = $4`,
		intentID, reason, StatusSuppressed, StatusPending)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, errcode.ErrConflict
	}
	return readIntentTx(ctx, tx, intentID)
}

func readIntentTx(ctx context.Context, tx pgx.Tx, intentID int64) (*Intent, error) {
	return scanIntent(tx.QueryRow(ctx, `SELECT `+intentCols+`
		FROM riskreminder_intents WHERE id = $1`, intentID))
}

// readIntentRow 投递事务外的独立读回（提交确认仲裁用）。
func readIntentRow(ctx context.Context, db *pgxpool.Pool, intentID int64) (*Intent, error) {
	return scanIntent(db.QueryRow(ctx, `SELECT `+intentCols+`
		FROM riskreminder_intents WHERE id = $1`, intentID))
}

// recordAttemptFailure 投递失败的独立留痕（原事务已回滚）：attempt_count+1、
// last_error、last_attempted_at——「失败可按原键恢复并明确记录」；留痕失败仅记日志
// （例如整库不可用时无从落库，意图本身也仍在 pending，恢复后重投）。
func (s *Service) recordAttemptFailure(ctx context.Context, intentID int64, cause error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		s.logger.Warn("riskreminder: attempt bookkeeping begin failed", "intent", intentID, "err", err)
		return
	}
	if _, err := tx.Exec(ctx, `
		UPDATE riskreminder_intents
		SET attempt_count = attempt_count + 1, last_error = $2, last_attempted_at = now()
		WHERE id = $1 AND status = $3`,
		intentID, truncateRunes(cause.Error(), 512), StatusPending); err != nil {
		_ = tx.Rollback(ctx)
		s.logger.Warn("riskreminder: attempt bookkeeping failed", "intent", intentID, "err", err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		s.logger.Warn("riskreminder: attempt bookkeeping commit failed", "intent", intentID, "err", err)
	}
}

func displayTicketKey(ticketID int64, humanKey *string) string {
	if humanKey != nil && *humanKey != "" {
		return *humanKey
	}
	return "#" + strconv.FormatInt(ticketID, 10)
}

// truncateRunes 按字节预算截断（不破 UTF-8 序列，含省略号一并计入预算）——
// suppress_reason/last_error 列有长度约束，超长原因在 Go 侧收敛而非 22001 报错。
func truncateRunes(s string, maxBytes int) string {
	const ellipsis = "…"
	if len(s) <= maxBytes {
		return s
	}
	budget := maxBytes - len(ellipsis)
	if budget <= 0 {
		return ellipsis
	}
	cut := 0
	for idx, r := range s {
		size := utf8.RuneLen(r)
		if size < 1 {
			size = 1 // 替换符兜底——不推进坏序列
		}
		if idx+size > budget {
			break
		}
		cut = idx + size
	}
	return s[:cut] + ellipsis
}

// S6 二期 B6-7 站内消息中心（06 号 §6——事件系统消费者：inbox 通道+读面）。
// notifications = 事件快照（读过即完）；title/link 入站即渲染（读面零渲染逻辑）；
// 未读数并入 S10 角标 sections（badge_service 计算器注册制——phase6 零改协议）。
// 保留策略：N 天封顶物理清理（骑 CronScheduler——复用 message_log 经验）。
package repository

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
)

type NotificationInboxRepo struct {
	db *pgxpool.Pool
}

const NeutralTicketNotificationTitle = "请查看工单"

// The numeric ticket identity comes from the event or the notification row.
// Ended groups still count. A successful dispatch's immutable receipt is the
// numeric tombstone once the group's ticket FK is set to NULL on deletion.
// Keep the receipt lookup off the live-ticket path.
const riskTicketNotificationSQL = `CASE
	WHEN notice.ticket_id IS NULL OR notice.ticket_id <= 0 THEN false
	WHEN EXISTS (SELECT 1 FROM risk_dispatch_groups g WHERE g.ticket_id = notice.ticket_id) THEN true
	WHEN EXISTS (SELECT 1 FROM tickets t WHERE t.id = notice.ticket_id) THEN false
	ELSE EXISTS (SELECT 1 FROM risk_dispatch_group_receipts r
		WHERE r.result->>'ticket_id' = notice.ticket_id::text)
END`

func NewNotificationInboxRepo(db *pgxpool.Pool) *NotificationInboxRepo {
	return &NotificationInboxRepo{db: db}
}

// Insert 落一条站内通知。B2 工单消息入站时只持久化中性文本和平台深链。
func (r *NotificationInboxRepo) Insert(ctx context.Context, n *model.NotificationRow) error {
	return r.db.QueryRow(ctx, `
		INSERT INTO notifications (recipient_id, biz_type, biz_id, title, link)
		SELECT $1, $2::text, notice.ticket_id,
		       CASE WHEN policy.restricted THEN $6::text ELSE $4::text END,
		       CASE WHEN policy.restricted THEN '/tickets/' || notice.ticket_id::text ELSE NULLIF($5::text, '') END
		FROM (SELECT $3::bigint AS ticket_id) notice
		CROSS JOIN LATERAL (SELECT CASE WHEN $2::text = 'ticket' THEN `+riskTicketNotificationSQL+` ELSE false END AS restricted) policy
		RETURNING id, created_at`,
		n.RecipientID, n.BizType, n.BizID, n.Title, n.Link, NeutralTicketNotificationTitle,
	).Scan(&n.ID, &n.CreatedAt)
}

// InsertTx Insert 的事务版（同 SQL 同语义，仅换执行载体）——调用方需要
// 入箱与其成功事实同事务提交时使用（phase6 B2 接单超时提醒：一次性效果）。
func (r *NotificationInboxRepo) InsertTx(ctx context.Context, tx pgx.Tx, n *model.NotificationRow) error {
	return tx.QueryRow(ctx, `
		INSERT INTO notifications (recipient_id, biz_type, biz_id, title, link)
		VALUES ($1, $2, $3, $4, NULLIF($5, ''))
		RETURNING id, created_at`,
		n.RecipientID, n.BizType, n.BizID, n.Title, n.Link,
	).Scan(&n.ID, &n.CreatedAt)
}

// Inbox 收件箱分页（倒序；unread 过滤可选）。
func (r *NotificationInboxRepo) Inbox(ctx context.Context, recipientID int64, unreadOnly bool, page, pageSize int) ([]*model.NotificationRow, int64, error) {
	where := `WHERE recipient_id = $1`
	args := []any{recipientID}
	if unreadOnly {
		where += ` AND read_at IS NULL`
	}
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT COUNT(*) FROM notifications `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	listArgs := append(append([]any{}, args...), pageSize, (page-1)*pageSize)
	rows, err := r.db.Query(ctx, `
		SELECT id, recipient_id, biz_type, biz_id, COALESCE(title, ''), COALESCE(link, ''), created_at, read_at,
		       CASE WHEN biz_type = 'ticket' THEN `+riskTicketNotificationSQL+` ELSE false END AS restricted,
		       EXISTS (SELECT 1 FROM riskreminder_intents i
			       WHERE i.ticket_id = notice.ticket_id AND i.notification_id = notice.id) AS accept_timeout
		FROM (SELECT notifications.*, biz_id AS ticket_id FROM notifications `+where+`
		      ORDER BY id DESC LIMIT $`+strconv.Itoa(len(args)+1)+` OFFSET $`+strconv.Itoa(len(args)+2)+`) notice
		ORDER BY id DESC`, listArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (*model.NotificationRow, error) {
		var n model.NotificationRow
		var restricted, acceptTimeout bool
		if err := row.Scan(&n.ID, &n.RecipientID, &n.BizType, &n.BizID, &n.Title, &n.Link, &n.CreatedAt, &n.ReadAt, &restricted, &acceptTimeout); err != nil {
			return nil, err
		}
		if restricted {
			n.Title = NeutralTicketNotificationTitle
			if acceptTimeout {
				n.Title = "工单超时未接单"
			}
			n.Link = "/tickets/" + strconv.FormatInt(*n.BizID, 10)
		}
		return &n, nil
	})
	return list, total, err
}

// MarkRead 单条已读（recipient 归属校验——非本人 404 同形）。
func (r *NotificationInboxRepo) MarkRead(ctx context.Context, id, recipientID int64) error {
	ct, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE id = $1 AND recipient_id = $2 AND read_at IS NULL`,
		id, recipientID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		// 已读/不存在——幂等语义返回 nil（读面操作无副作用差异）
		var exists bool
		if err := r.db.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM notifications WHERE id = $1 AND recipient_id = $2)`, id, recipientID).Scan(&exists); err != nil {
			return err
		}
		if !exists {
			return errcode.New(errcode.ErrNotFound.Code, "通知不存在")
		}
	}
	return nil
}

// MarkAllRead 全部已读（批量——A.8.5 第一档）。
func (r *NotificationInboxRepo) MarkAllRead(ctx context.Context, recipientID int64) (int64, error) {
	ct, err := r.db.Exec(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE recipient_id = $1 AND read_at IS NULL`, recipientID)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// UnreadCount 未读数（badges sections.unread 数据源）。
func (r *NotificationInboxRepo) UnreadCount(ctx context.Context, recipientID int64) (int64, error) {
	var n int64
	err := r.db.QueryRow(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND read_at IS NULL`, recipientID).Scan(&n)
	return n, err
}

// CleanupExpired 保留策略（N 天封顶物理清理——骑 CronScheduler）。
func (r *NotificationInboxRepo) CleanupExpired(ctx context.Context, retentionDays int) (int64, error) {
	ct, err := r.db.Exec(ctx,
		`DELETE FROM notifications WHERE created_at < NOW() - make_interval(days => $1)`, retentionDays)
	if err != nil {
		return 0, err
	}
	return ct.RowsAffected(), nil
}

// MessageLogRepo 发送记录仓库 + MessageRuleRepo 订阅规则仓库（S6 B6-6）。
package repository

import (
	"context"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
)

type MessageLogRepo struct {
	db *pgxpool.Pool
}

func NewMessageLogRepo(db *pgxpool.Pool) *MessageLogRepo {
	return &MessageLogRepo{db: db}
}

// Insert 落一行发送记录（notify hook 每次发送后调用——失败也落行记录 status=failed）。
func (r *MessageLogRepo) Insert(ctx context.Context, log *model.MessageLog) error {
	_, err := r.db.Exec(ctx, `
		INSERT INTO message_log (template_code, biz_type, biz_id, channel, recipients, subject, body, status, error, actor_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NULLIF($9, ''), $10)`,
		log.TemplateCode, log.BizType, log.BizID, log.Channel, log.Recipients,
		log.Subject, log.Body, log.Status, log.Error, log.ActorID)
	return err
}

// List 分页列表（管理面）。
func (r *MessageLogRepo) List(ctx context.Context, page, pageSize int, bizType, tplCode string) ([]*model.MessageLog, int64, error) {
	where := "WHERE 1=1"
	args := []any{}
	if bizType != "" {
		args = append(args, bizType)
		where += ` AND biz_type = $` + strconv.Itoa(len(args))
	}
	if tplCode != "" {
		args = append(args, tplCode)
		where += ` AND template_code = $` + strconv.Itoa(len(args))
	}
	var total int64
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM message_log `+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	args = append(args, pageSize, (page-1)*pageSize)
	q := `SELECT id, template_code, biz_type, biz_id, channel, recipients, subject, body, status,
		COALESCE(error, ''), COALESCE(actor_id, 0), created_at
		FROM message_log ` + where + ` ORDER BY id DESC LIMIT $` + strconv.Itoa(len(args)-1) + ` OFFSET $` + strconv.Itoa(len(args))
	rows, err := r.db.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []*model.MessageLog
	for rows.Next() {
		var l model.MessageLog
		var actor int64
		if err := rows.Scan(&l.ID, &l.TemplateCode, &l.BizType, &l.BizID, &l.Channel,
			&l.Recipients, &l.Subject, &l.Body, &l.Status, &l.Error, &actor, &l.CreatedAt); err != nil {
			return nil, 0, err
		}
		if actor > 0 {
			l.ActorID = &actor
		}
		out = append(out, &l)
	}
	return out, total, nil
}

// ── MessageRuleRepo ──

type MessageRuleRepo struct {
	db *pgxpool.Pool
}

func NewMessageRuleRepo(db *pgxpool.Pool) *MessageRuleRepo {
	return &MessageRuleRepo{db: db}
}

// GetByAction 按事件动作查启用规则（notify hook 消费——替代硬编码映射）。
func (r *MessageRuleRepo) GetByAction(ctx context.Context, bizType, eventAction string) (*model.MessageRule, error) {
	var rule model.MessageRule
	err := r.db.QueryRow(ctx, `
		SELECT id, biz_type, event_action, template_code, recipient_type,
			COALESCE(fixed_recipients, ''), enabled, version, COALESCE(created_by, 0), created_at, updated_at
		FROM message_rules WHERE biz_type = $1 AND event_action = $2 AND enabled = true`,
		bizType, eventAction).Scan(
		&rule.ID, &rule.BizType, &rule.EventAction, &rule.TemplateCode, &rule.RecipientType,
		&rule.FixedRecipients, &rule.Enabled, &rule.Version, &rule.CreatedBy, &rule.CreatedAt, &rule.UpdatedAt)
	if err == pgx.ErrNoRows {
		return nil, nil // 无规则=不通知（非错误）
	}
	if err != nil {
		return nil, err
	}
	return &rule, nil
}

// ListRules 分页列表（管理面）。
func (r *MessageRuleRepo) ListRules(ctx context.Context) ([]*model.MessageRule, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, biz_type, event_action, template_code, recipient_type,
			COALESCE(fixed_recipients, ''), enabled, version, COALESCE(created_by, 0), created_at, updated_at
		FROM message_rules ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*model.MessageRule
	for rows.Next() {
		var rule model.MessageRule
		var createdBy int64
		if err := rows.Scan(&rule.ID, &rule.BizType, &rule.EventAction, &rule.TemplateCode,
			&rule.RecipientType, &rule.FixedRecipients, &rule.Enabled, &rule.Version, &createdBy,
			&rule.CreatedAt, &rule.UpdatedAt); err != nil {
			return nil, err
		}
		if createdBy > 0 {
			rule.CreatedBy = &createdBy
		}
		out = append(out, &rule)
	}
	return out, nil
}

// ── NotificationOptoutRepo（B6-4——per-user opt-out/免打扰）──

type NotificationOptoutRepo struct {
	db *pgxpool.Pool
}

func NewNotificationOptoutRepo(db *pgxpool.Pool) *NotificationOptoutRepo {
	return &NotificationOptoutRepo{db: db}
}

// IsOptedOut 检查用户是否对指定事件静默（精确 action 或 '*' 全域免打扰）。
func (r *NotificationOptoutRepo) IsOptedOut(ctx context.Context, userID int64, bizType, eventAction string) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM notification_optouts
			WHERE user_id = $1 AND biz_type = $2 AND event_action IN ($3, '*'))`,
		userID, bizType, eventAction).Scan(&exists)
	return exists, err
}

// DeleteRuleByAction 按事件动作删规则（测试清理面）。
func (r *MessageRuleRepo) DeleteRuleByAction(ctx context.Context, bizType, eventAction string) error {
	_, err := r.db.Exec(ctx, `DELETE FROM message_rules WHERE biz_type=$1 AND event_action=$2`, bizType, eventAction)
	return err
}

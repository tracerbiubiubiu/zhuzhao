// MessageAdminService 消息管理服务层（S6 B6-6——handler 经 service 访问 repo）。
package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type MessageAdminService struct {
	logRepo  *repository.MessageLogRepo
	ruleRepo *repository.MessageRuleRepo
	pool     *pgxpool.Pool
}

func NewMessageAdminService(pool *pgxpool.Pool) *MessageAdminService {
	return &MessageAdminService{
		logRepo:  repository.NewMessageLogRepo(pool),
		ruleRepo: repository.NewMessageRuleRepo(pool),
		pool:     pool,
	}
}

func (s *MessageAdminService) ListLogs(ctx context.Context, page, pageSize int, bizType, tplCode string) ([]*model.MessageLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	return s.logRepo.List(ctx, page, pageSize, bizType, tplCode)
}

func (s *MessageAdminService) ListRules(ctx context.Context) ([]*model.MessageRule, error) {
	return s.ruleRepo.ListRules(ctx)
}

func (s *MessageAdminService) UpsertRule(ctx context.Context, rule *model.MessageRule) error {
	if rule.BizType == "" {
		rule.BizType = "ticket"
	}
	if rule.RecipientType == "" {
		rule.RecipientType = "assignee"
	}
	_, err := s.pool.Exec(ctx, `
		INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type, fixed_recipients, enabled, created_by)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7)
		ON CONFLICT (biz_type, event_action) DO UPDATE SET
			template_code = EXCLUDED.template_code,
			recipient_type = EXCLUDED.recipient_type,
			fixed_recipients = EXCLUDED.fixed_recipients,
			enabled = EXCLUDED.enabled,
			updated_at = NOW()`,
		rule.BizType, rule.EventAction, rule.TemplateCode, rule.RecipientType,
		rule.FixedRecipients, rule.Enabled, rule.CreatedBy)
	return err
}

func (s *MessageAdminService) DeleteRule(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM message_rules WHERE id = $1`, id)
	return err
}

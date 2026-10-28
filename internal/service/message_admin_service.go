// MessageAdminService 消息管理服务层（S6 B6-6——handler 经 service 访问 repo）。
package service

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/tracerbiubiubiu/zhuzhao/internal/model"
	"github.com/tracerbiubiubiu/zhuzhao/internal/pkg/errcode"
	"github.com/tracerbiubiubiu/zhuzhao/internal/repository"
)

type MessageAdminService struct {
	logRepo  *repository.MessageLogRepo
	ruleRepo *repository.MessageRuleRepo
	tplRepo  *repository.MessageTemplateRepo
	pool     *pgxpool.Pool
}

func NewMessageAdminService(pool *pgxpool.Pool) *MessageAdminService {
	return &MessageAdminService{
		logRepo:  repository.NewMessageLogRepo(pool),
		ruleRepo: repository.NewMessageRuleRepo(pool),
		tplRepo:  repository.NewMessageTemplateRepo(pool),
		pool:     pool,
	}
}

func (s *MessageAdminService) ListLogs(ctx context.Context, page, pageSize int, bizType, tplCode, status, bizID string) ([]*model.MessageLog, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	if status != "" && status != "sent" && status != "failed" {
		return nil, 0, errcode.New(errcode.ErrInvalidParams.Code, "status 须为 sent/failed")
	}
	return s.logRepo.List(ctx, page, pageSize, bizType, tplCode, status, bizID)
}

func (s *MessageAdminService) ListRules(ctx context.Context) ([]*model.MessageRule, error) {
	return s.ruleRepo.ListRules(ctx)
}

// validateUpsertRule F-2-4 修复：规则保存四项校验（外审发现的越权外发路径）。
func (s *MessageAdminService) validateUpsertRule(ctx context.Context, rule *model.MessageRule) error {
	// ① 模板存在且启用
	tpl, err := s.tplRepo.GetByCode(ctx, rule.TemplateCode)
	if err != nil {
		return fmt.Errorf("模板 %s 不存在或未启用: %w", rule.TemplateCode, err)
	}
	_ = tpl

	// ② recipient_type 枚举（v3 审计 Y-1：裸 fmt.Errorf → errcode 400，修复前 errors.As
	// 落空 → 500+10000 污染 5xx 告警面）
	switch rule.RecipientType {
	case "assignee", "creator", "fixed", "cc": // cc=S10 B10-1（payload cc_users——抄送通知档）
	default:
		return errcode.New(errcode.ErrInvalidParams.Code,
			fmt.Sprintf("recipient_type %q 不合法（assignee/creator/fixed/cc）", rule.RecipientType))
	}

	// ③ event_action 词表校验（ticket. 前缀 + 已知动作）
	if !isValidEventAction(rule.EventAction) {
		return errcode.New(errcode.ErrInvalidParams.Code,
			fmt.Sprintf("event_action %q 不在支持词表", rule.EventAction))
	}

	// ④ fixed 收件人非空时验格式（逗号分隔数字 ID）
	if rule.RecipientType == "fixed" && rule.FixedRecipients != "" {
		for _, id := range strings.Split(rule.FixedRecipients, ",") {
			if _, err := strconv.ParseInt(strings.TrimSpace(id), 10, 64); err != nil {
				return errcode.New(errcode.ErrInvalidParams.Code,
					fmt.Sprintf("fixed_recipients 含非数字 ID %q", strings.TrimSpace(id)))
			}
		}
	}
	return nil
}

var validEventActions = map[string]bool{
	"ticket.created": true, "ticket.assigned": true, "ticket.closed": true,
	"ticket.updated": true, "ticket.commented": true,
	"ticket.cc_added":        true, // S10 B10-1（种子规则 000062 同款——管理员可改路由）
	"ticket.timeout":         true, // S10 B10-4 超时催办（种子规则 000064 同款）
	"ops.cron_failed":        true, // S10 B10-4 cron 失败告警（BizType=ops——000064 种子）
	"ticket.transitioned":    true, // S3a B3-1（无种子规则=默认不通知——管理员可配）
	"ticket.dispatch_missed": true, // S3a B3-2（新单无人接监控——管理员可配告警路由）
	"ticket.deadline":        true, // S3a SLA 层 0.5（deadline 到期——000071 种子规则）
	"ticket.action_notify":   true, // S3a B3-3 notify 动作（000079 种子规则；动作 template 参数可逐次覆盖）
	"ops.action_failed":      true, // S3a B3-3 动作失败告警（G-5——payload.actor 锚点，规则 creator/fixed 档可路由）
}

func isValidEventAction(action string) bool { return validEventActions[action] }

func (s *MessageAdminService) UpsertRule(ctx context.Context, rule *model.MessageRule) error {
	if err := s.validateUpsertRule(ctx, rule); err != nil {
		return err
	}
	if rule.BizType == "" {
		rule.BizType = "ticket"
	}
	if rule.RecipientType == "" {
		rule.RecipientType = "assignee"
	}
	// version CAS（000060 乐观锁——对齐 message_templates/system_settings 全仓惯例）：
	// 新建=无冲突直接 INSERT（version 起步 1，入参 version 忽略）；更新=DO UPDATE
	// WHERE version 命中才写并自增；陈旧 version → 0 行 → 92506/409 不静默覆盖。
	tag, err := s.pool.Exec(ctx, `
		INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type, fixed_recipients, enabled, created_by)
		VALUES ($1, $2, $3, $4, NULLIF($5,''), $6, $7)
		ON CONFLICT (biz_type, event_action) DO UPDATE SET
			template_code = EXCLUDED.template_code,
			recipient_type = EXCLUDED.recipient_type,
			fixed_recipients = EXCLUDED.fixed_recipients,
			enabled = EXCLUDED.enabled,
			updated_at = NOW(),
			version = message_rules.version + 1
		WHERE message_rules.version = $8`,
		rule.BizType, rule.EventAction, rule.TemplateCode, rule.RecipientType,
		rule.FixedRecipients, rule.Enabled, rule.CreatedBy, rule.Version)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errcode.ErrMsgRuleVersionConflict
	}
	return nil
}

func (s *MessageAdminService) DeleteRule(ctx context.Context, id int64) error {
	_, err := s.pool.Exec(ctx, `DELETE FROM message_rules WHERE id = $1`, id)
	return err
}

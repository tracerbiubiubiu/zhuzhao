-- S3a 随窗件 SLA 层 0.5（03 号 §5 四层路径——deadline 模型 A，gap2 确认 W15 顺带）：
-- tickets.sla_due_at 列复活（模型 A 残留死字段激活——P4 期从不写入）+
-- transition_meta state 级 sla_due_hours 键（与 timeout_minutes 同段共存）+
-- 过期部分索引（deadline 扫描——timeout_scanner 同型）+ 种子模板/规则。
-- 语义：类型按状态配 sla_due_hours → Create/Transition 写 due 刻 →
-- deadline_scan（骑 CronScheduler）到点 emit ticket.deadline（通知走 S6 规则面）。
ALTER TABLE tickets ALTER COLUMN sla_due_at SET DEFAULT NULL; -- no-op 语义对齐（列已存在）
UPDATE tickets SET sla_due_at = NULL WHERE sla_due_at IS NOT NULL; -- 存量死数据清零（从不写入——零行预期）
CREATE INDEX IF NOT EXISTS idx_tickets_deadline ON tickets (sla_due_at)
    WHERE sla_due_at IS NOT NULL AND status NOT IN ('closed', 'rejected');

INSERT INTO message_templates (code, name, channel, biz_type, subject, body, created_by) VALUES
    ('ticket_deadline_notify', '工单到期提醒', 'uip', 'ticket',
     '[{{ticket.human_key}}] 工单已到处理期限：{{ticket.title}}',
     '工单 {{ticket.title}}（{{ticket.human_key}}）已到配置的处理期限（状态 {{ticket.status}}）。\n链接：{{ticket.link}}',
     NULL)
ON CONFLICT (code) DO NOTHING;

INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type) VALUES
    ('ticket', 'ticket.deadline', 'ticket_deadline_notify', 'cc')
ON CONFLICT (biz_type, event_action) DO NOTHING;

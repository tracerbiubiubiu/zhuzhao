DELETE FROM message_rules WHERE biz_type = 'ticket' AND event_action = 'ticket.deadline';
DELETE FROM message_templates WHERE code = 'ticket_deadline_notify';
DROP INDEX IF EXISTS idx_tickets_deadline;
-- sla_due_at 列保留（历史列——down 不删防误伤既有读侧）；数据清零还原死存储态
UPDATE tickets SET sla_due_at = NULL;

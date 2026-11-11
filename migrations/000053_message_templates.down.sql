-- 000053 回滚：删种子行→删表（模板无外部 FK——message_rules 尚未建）
DELETE FROM message_templates WHERE code IN ('ticket_created_notify', 'ticket_assigned_notify', 'ticket_closed_notify');
DROP TABLE IF EXISTS message_templates;

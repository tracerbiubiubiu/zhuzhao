-- 000064 down：对称回滚——menu_apis 来源行 → 种子规则/模板（FK 顺序）→ 表/列。
-- ⚠ status_changed_at 列回滚即丢进入时间数据（语义列无外部依赖，down 幂等可重放）。
DELETE FROM menu_apis WHERE api_path = '/api/v1/system/cron-status' AND api_method = 'GET';
DELETE FROM message_rules WHERE biz_type = 'ops' AND event_action = 'ops.cron_failed';
DELETE FROM message_rules WHERE biz_type = 'ticket' AND event_action = 'ticket.timeout';
DELETE FROM message_templates WHERE code IN ('ops_cron_alert', 'ticket_timeout_notify');
DROP INDEX IF EXISTS idx_tickets_timeout;
DROP TABLE IF EXISTS ticket_reminders;
ALTER TABLE tickets DROP COLUMN IF EXISTS escalation_supervisor_id;
ALTER TABLE tickets DROP COLUMN IF EXISTS status_changed_at;
ALTER TABLE ticket_types DROP COLUMN IF EXISTS transition_meta;

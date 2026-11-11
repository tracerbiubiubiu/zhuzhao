-- S10 B10-4 节点超时 + 催办（10 号 §3.4/附录 A.3/A.5——E-1/E-2 已拍 2026-10-24）：
-- ① ticket_types.transition_meta JSONB（state 级键 timeout_minutes/escalation——
--    S3a 后 "from>to" 转换级键复用同列，">" 区分键形）
-- ② tickets.status_changed_at（E-2 拍定 a 案：UpdateStatusTx 咽喉同写；存量回填
--    = updated_at 近似——首跑观察模式缓解误报，观察开关 config 化默认 true）
-- ③ tickets.escalation_supervisor_id（E-1 拍定改写：人工指定催办上级——dd §31
--    「人工指定为基线，HR 直属上级为进阶」；Create/Assign 可填可改）
-- ④ ticket_reminders 催办水位侧表（F-3-7 拍定：每单每状态天然成对、O(1) 读、
--    避 JSONB 读改写并发）
-- ⑤ 超时部分索引（扫描只摸活跃单）+ 种子模板/规则（ticket.timeout 告警链）
ALTER TABLE ticket_types ADD COLUMN IF NOT EXISTS transition_meta JSONB NOT NULL DEFAULT '{}';

ALTER TABLE tickets ADD COLUMN IF NOT EXISTS status_changed_at TIMESTAMPTZ;
-- 存量回填（一次性）：updated_at 被评论/编辑污染=近似值——首跑观察模式只记日志
-- 不发通知，观察开关确认无异常后关闭（config internal_jobs.timeout_observe）
UPDATE tickets SET status_changed_at = updated_at WHERE status_changed_at IS NULL;

ALTER TABLE tickets ADD COLUMN IF NOT EXISTS escalation_supervisor_id BIGINT REFERENCES users(id);

CREATE TABLE IF NOT EXISTS ticket_reminders (
    id               BIGSERIAL PRIMARY KEY,
    ticket_id        BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    state            VARCHAR(64) NOT NULL,
    reminder_count   INT NOT NULL DEFAULT 0,
    last_reminder_at TIMESTAMPTZ,
    UNIQUE (ticket_id, state)
);

CREATE INDEX IF NOT EXISTS idx_tickets_timeout ON tickets (status, status_changed_at)
    WHERE status NOT IN ('closed', 'rejected');

-- 种子：超时催办通知（cc_users=escalation notify 解析出的全部目标——处理人∪上级）
INSERT INTO message_templates (code, name, channel, biz_type, subject, body, created_by) VALUES
    ('ticket_timeout_notify', '工单超时催办', 'uip', 'ticket',
     '[{{ticket.human_key}}] 工单超时：{{ticket.title}}',
     '工单 {{ticket.title}}（{{ticket.human_key}}）在状态 {{ticket.status}} 停留已超时。\n已催办 {{ticket.reminder_count}} 次\n链接：{{ticket.link}}',
     NULL)
ON CONFLICT (code) DO NOTHING;

INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type) VALUES
    ('ticket', 'ticket.timeout', 'ticket_timeout_notify', 'cc')
ON CONFLICT (biz_type, event_action) DO NOTHING;

-- 种子：cron 任务失败告警模板（ops 域——收件人 fixed 由管理员在消息管理面配置，
-- 空=暂不发送；事件 BizType=ops 域边界显式化）
INSERT INTO message_templates (code, name, channel, biz_type, subject, body, created_by) VALUES
    ('ops_cron_alert', '定时任务失败告警', 'uip', 'ops',
     '[ops] 定时任务 {{event.job}} 失败',
     '定时任务 {{event.job}} 执行失败：{{event.error}}\n上次成功：{{event.last_success}}',
     NULL)
ON CONFLICT (code) DO NOTHING;

INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type, fixed_recipients) VALUES
    ('ops', 'ops.cron_failed', 'ops_cron_alert', 'fixed', '')
ON CONFLICT (biz_type, event_action) DO NOTHING;

-- cron 状态只读端点 menu_apis 绑定（audit_logs 菜单行——运维观测面随审计页族；
-- L1 走既有 audit:read 码，admin/superadmin 通配可达）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/system/cron-status', 'GET'
FROM menus m WHERE m.code = 'audit_log' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = '/api/v1/system/cron-status' AND ma.api_method = 'GET');

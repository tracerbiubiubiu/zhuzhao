-- S10 B10-1 工单抄送表（10 号附录 A.3 ①——拍板点 2 预确认：S10 先行 → role 语义
-- 预留 cc/participant，防 phase3/10 §10.1 ticket_participants 同语义双表）。
-- L2 特例通道（拍板点 1 预确认「抄送即可见」）：GetFilter 第四 OR 支反查
-- idx_ticket_cc_user（resource.go ccVisibilitySQL 同批）。
CREATE TABLE IF NOT EXISTS ticket_cc (
    id         BIGSERIAL PRIMARY KEY,
    ticket_id  BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id),
    role       VARCHAR(20) NOT NULL DEFAULT 'cc',   -- 预留：cc / participant（§3.1 对表项）
    created_by BIGINT NOT NULL REFERENCES users(id), -- 谁加的抄送（操作留痕）
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (ticket_id, user_id, role)
);
CREATE INDEX IF NOT EXISTS idx_ticket_cc_user ON ticket_cc (user_id);

-- 种子模板 + 种子规则（S6 06 号 §4/§5.2 同款——抄送通知走 S6 规则面零新通知代码；
-- 管理员可经消息管理面改路由/收件人档）
INSERT INTO message_templates (code, name, channel, biz_type, subject, body, created_by) VALUES
    ('ticket_cc_notify', '工单抄送通知', 'uip', 'ticket',
     '[{{ticket.human_key}}] 工单抄送给您：{{ticket.title}}',
     '工单 {{ticket.title}}（{{ticket.human_key}}）已抄送给您。\n类型：{{ticket.type_code}}\n状态：{{ticket.status}}\n链接：{{ticket.link}}',
     NULL)
ON CONFLICT (code) DO NOTHING;

INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type) VALUES
    ('ticket', 'ticket.cc_added', 'ticket_cc_notify', 'cc')
ON CONFLICT (biz_type, event_action) DO NOTHING;

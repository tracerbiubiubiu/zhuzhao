-- S6 B6-6 发送记录 + 订阅规则（06 号 §5.4/A.5）+ menu_apis 正式绑定
-- ① message_log：每次 ChannelSender.Send 调用落行（模板/收件人/状态/错误）
-- ② message_rules：订阅规则轻表（状态→模板路由）
-- ③ menu_apis：模板管理四端点正式绑定（撤 catalogExempt 过渡态）

-- ① 发送记录表
CREATE TABLE IF NOT EXISTS message_log (
    id            BIGSERIAL PRIMARY KEY,
    template_code VARCHAR(64)  NOT NULL,               -- 模板标识（审计归因）
    biz_type      VARCHAR(32)  NOT NULL,                -- 业务域
    biz_id        VARCHAR(64)  NOT NULL,                -- 业务对象 ID
    channel       VARCHAR(32)  NOT NULL,                -- 渠道
    recipients    TEXT         NOT NULL,                -- 逗号分隔收件人列表
    subject       TEXT         NOT NULL,                -- 渲染后主题
    body          TEXT         NOT NULL,                -- 渲染后正文
    status        VARCHAR(20)  NOT NULL DEFAULT 'sent', -- sent / failed
    error         TEXT,                                  -- 失败原因
    actor_id      BIGINT,                                -- 操作人（审计）
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_message_log_biz ON message_log (biz_type, biz_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_message_log_tpl ON message_log (template_code, created_at DESC);

-- ② 订阅规则轻表（06 号 A.5——状态→模板路由的管理面）
CREATE TABLE IF NOT EXISTS message_rules (
    id            BIGSERIAL PRIMARY KEY,
    biz_type      VARCHAR(32)  NOT NULL DEFAULT 'ticket',
    event_action  VARCHAR(64)  NOT NULL,               -- DomainEvent.Action（如 ticket.assigned）
    template_code VARCHAR(64)  NOT NULL,               -- 使用的模板
    recipient_type VARCHAR(32) NOT NULL DEFAULT 'assignee', -- 收件人类型（assignee/creator/fixed）
    fixed_recipients TEXT,                              -- recipient_type=fixed 时的收件人列表（逗号分隔）
    enabled       BOOLEAN      NOT NULL DEFAULT true,
    created_by    BIGINT,
    created_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    UNIQUE (biz_type, event_action)
);

-- 种子规则（与种子模板对齐——06 号 §5.2 默认路由）
INSERT INTO message_rules (biz_type, event_action, template_code, recipient_type) VALUES
    ('ticket', 'ticket.assigned', 'ticket_assigned_notify', 'assignee'),
    ('ticket', 'ticket.closed',   'ticket_closed_notify',   'assignee')
ON CONFLICT (biz_type, event_action) DO NOTHING;

-- ③ menu_apis 正式绑定（撤 catalogExempt 过渡态——B6-3 路由）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/message-templates', 'GET'
FROM menus m WHERE m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = '/api/v1/message-templates' AND ma.api_method = 'GET');

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, '/api/v1/message-templates', 'GET'
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM casbin_rule c WHERE c.ptype='p' AND c.v0='role::'||r.code AND c.v1='/api/v1/message-templates' AND c.v2='GET');

-- POST 三端点绑定（同 menu 同模式——B6-3 管理面写操作）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.path, 'POST'
FROM menus m, (VALUES ('/api/v1/message-templates'), ('/api/v1/message-templates/update'), ('/api/v1/message-templates/delete')) AS v(path)
WHERE m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.path AND ma.api_method = 'POST');

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, v.path, 'POST'
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
CROSS JOIN (VALUES ('/api/v1/message-templates'), ('/api/v1/message-templates/update'), ('/api/v1/message-templates/delete')) AS v(path)
WHERE NOT EXISTS (SELECT 1 FROM casbin_rule c WHERE c.ptype='p' AND c.v0='role::'||r.code AND c.v1=v.path AND c.v2='POST');

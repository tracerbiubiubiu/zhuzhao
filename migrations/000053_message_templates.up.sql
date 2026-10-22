-- S6 B6-3 消息模板表（06 号 §4——占位符替换 logic-less 零执行）
-- 迁移 000053：message_templates 表族（code 唯一/biz_type 路由/subject+body 模板）
CREATE TABLE IF NOT EXISTS message_templates (
    id              BIGSERIAL PRIMARY KEY,
    code            VARCHAR(64)  NOT NULL UNIQUE,         -- 模板标识（message_rules 引用锚）
    name            VARCHAR(128) NOT NULL,                 -- 展示名
    channel         VARCHAR(32)  NOT NULL DEFAULT 'uip',   -- 渠道（与 ChannelSender 对齐）
    biz_type        VARCHAR(32)  NOT NULL DEFAULT 'ticket', -- 业务域路由（DomainEvent.BizType）
    subject         VARCHAR(255) NOT NULL,                  -- 主题模板（含 {{}} 占位符）
    body            TEXT         NOT NULL,                  -- 正文模板（含 {{}} 占位符）
    variable_schema JSONB        DEFAULT NULL,              -- 可用变量声明（SSOT=服务端变量字典）
    enabled         BOOLEAN      NOT NULL DEFAULT true,
    version         BIGINT       NOT NULL DEFAULT 1,        -- 乐观锁
    created_by      BIGINT,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

-- 种子模板（06 号 §4：默认只带编号+标题+链接——防越 L2 泄露）
INSERT INTO message_templates (code, name, channel, biz_type, subject, body, created_by) VALUES
    ('ticket_created_notify', '工单创建通知', 'uip', 'ticket',
     '[{{ticket.human_key}}] 新工单：{{ticket.title}}',
     '您有一张新工单：{{ticket.title}}（{{ticket.human_key}}）\n类型：{{ticket.type_code}}\n链接：{{ticket.link}}',
     NULL),
    ('ticket_assigned_notify', '工单分派通知', 'uip', 'ticket',
     '[{{ticket.human_key}}] 工单分派给您',
     '工单 {{ticket.title}}（{{ticket.human_key}}）已分派给您。\n优先级：{{ticket.priority}}\n链接：{{ticket.link}}',
     NULL),
    ('ticket_closed_notify', '工单关闭通知', 'uip', 'ticket',
     '[{{ticket.human_key}}] 工单已关闭',
     '工单 {{ticket.title}}（{{ticket.human_key}}）已关闭。\n链接：{{ticket.link}}',
     NULL)
ON CONFLICT (code) DO NOTHING;

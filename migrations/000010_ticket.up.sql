-- 000010: 工单模块核心表（Phase 2a Step 2）
-- DDL SSOT: docs/modules/ticket.md §3 + docs/phase2/09-ticket.md §2
-- 5 张表：ticket_types / ticket_type_fields / tickets / ticket_comments / ticket_events

-- ① 工单类型配置表（配置即代码：新增类型无需改代码）
CREATE TABLE IF NOT EXISTS ticket_types (
    id                BIGSERIAL PRIMARY KEY,
    code              VARCHAR(50) UNIQUE NOT NULL,
    name              VARCHAR(100) NOT NULL,
    description       TEXT,
    states            JSONB NOT NULL DEFAULT '["open","assigned","in_progress","pending_verify","closed","rejected"]',
    transitions       JSONB NOT NULL DEFAULT '{"open":["assigned","closed"],"assigned":["in_progress","open"],"in_progress":["pending_verify","rejected","closed"],"pending_verify":["closed","in_progress"],"closed":["open"],"rejected":["open"]}',
    default_sla_hours INT DEFAULT 24,           -- 小时（类型级默认）；Phase 2a 仅存储，SLA 计时 Phase 3 启用
    has_custom_fields BOOLEAN DEFAULT FALSE,
    is_active         BOOLEAN DEFAULT TRUE,
    created_at        TIMESTAMPTZ DEFAULT NOW()
);

-- ② 工单类型字段定义（动态表单，前端按此渲染）
CREATE TABLE IF NOT EXISTS ticket_type_fields (
    id            BIGSERIAL PRIMARY KEY,
    type_code     VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    field_key     VARCHAR(50) NOT NULL,
    field_label   VARCHAR(100) NOT NULL,
    field_type    VARCHAR(20) NOT NULL,          -- text/number/select/date/textarea
    field_options JSONB DEFAULT '[]',
    required      BOOLEAN DEFAULT FALSE,
    sort_order    INT DEFAULT 0,
    UNIQUE(type_code, field_key)
);

-- ③ 工单主表（所有类型共用，type_code 区分）
CREATE TABLE IF NOT EXISTS tickets (
    id           BIGSERIAL PRIMARY KEY,
    type_code    VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    title        VARCHAR(200) NOT NULL,
    description  TEXT,
    priority     SMALLINT DEFAULT 3,             -- 1紧急 2高 3中 4低
    status       VARCHAR(20) DEFAULT 'open',     -- open/assigned/in_progress/pending_verify/closed/rejected
    created_by   BIGINT NOT NULL,
    assigned_to BIGINT,
    org_id       BIGINT NOT NULL REFERENCES organizations(id),
    org_path     ltree NOT NULL,                  -- 冗余组织路径（2b ltree 过滤用）
    custom_data  JSONB DEFAULT '{}',
    sla_due_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_tickets_type_status ON tickets (type_code, status);
CREATE INDEX IF NOT EXISTS idx_tickets_org_path    ON tickets USING GIST (org_path);
CREATE INDEX IF NOT EXISTS idx_tickets_status      ON tickets (status);
CREATE INDEX IF NOT EXISTS idx_tickets_assigned    ON tickets (assigned_to);
CREATE INDEX IF NOT EXISTS idx_tickets_created     ON tickets (created_by);
CREATE INDEX IF NOT EXISTS idx_tickets_org_status  ON tickets (org_id, status);

-- ④ 工单回复/备注表
CREATE TABLE IF NOT EXISTS ticket_comments (
    id          BIGSERIAL PRIMARY KEY,
    ticket_id   BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL,
    content     TEXT NOT NULL,
    is_internal BOOLEAN DEFAULT FALSE,            -- 内部备注 vs 公开回复
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_comments_ticket ON ticket_comments (ticket_id, created_at);

-- ⑤ 工单事件日志（双重职责：审计日志 + Phase 3 事件队列）
-- Phase 3 迁移 000021 会加 event_type(audit/signal) + processed 列
CREATE TABLE IF NOT EXISTS ticket_events (
    id         BIGSERIAL PRIMARY KEY,
    ticket_id  BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL,
    action     VARCHAR(50) NOT NULL,              -- created/assigned/status_changed/closed
    from_value VARCHAR(50),
    to_value   VARCHAR(50),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_events_ticket ON ticket_events (ticket_id, created_at);

-- ⑥ 工单类型种子数据（2a）
INSERT INTO ticket_types (code, name, description, default_sla_hours) VALUES
    ('incident', '故障事件', '系统故障、服务中断等突发事件', 4),
    ('request',  '服务请求', '用户发起的标准化服务请求', 24)
ON CONFLICT (code) DO NOTHING;

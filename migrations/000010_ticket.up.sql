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

-- ========================================================================
-- ⑦ 工单管理菜单（catalog/page/button 三层）+ menu_apis + 角色绑定
--    SSOT: docs/phase2/09-ticket.md §1.1 + D2: ticket:list/create/read/update/close/assign/delete/comment/note
--    menu_type = SMALLINT (1=目录 2=页面 3=按钮)，parent_id 通过 code JOIN 反查
--    操作类 POST 路径不含 :id（id 放 body）；角色绑定页面即获该页全部 menu_apis；按钮菜单用于 UI 可见性 + AssignMenus 细粒度授权
-- ========================================================================

-- ⑦-a 目录 + 页面菜单（level 1/2）
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES
    -- level 1：工单管理大目录（parent_id=NULL，挂系统根；sort_order 与 Phase 1 system 目录（1）错开，取 2）
    ('ticket_manage', '工单管理', NULL, 1, '/tickets',  '',      'ticket',  NULL,          2, true),
    -- level 2：工单列表页面（permission = ticket:list 对应 R8/L1 Casbin 资源码）
    ('ticket_list',   '工单列表', NULL, 2, '/tickets',  'ticket/list/index', 'ticket-list', 'ticket:list', 1, true)
ON CONFLICT (code) DO NOTHING;

-- 回填页面菜单 parent_id（catalog → page 关联；两步 INSERT 避免序列 ID 硬编码）
UPDATE menus SET parent_id = (SELECT id FROM menus WHERE code = 'ticket_manage')
WHERE code = 'ticket_list' AND parent_id IS NULL;

-- ⑦-b 按钮菜单（level 3，9 动作 = list/create/read/update/close/assign/delete/comment/note）
INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, v.permission, v.sort_order, true
FROM (VALUES
    ('ticket_list', 'ticket_list_btn',    '查看工单', 'ticket:list',    1),
    ('ticket_list', 'ticket_create_btn',  '新建工单', 'ticket:create',  2),
    ('ticket_list', 'ticket_read_btn',    '工单详情', 'ticket:read',    3),
    ('ticket_list', 'ticket_update_btn',  '编辑工单', 'ticket:update',  4),
    ('ticket_list', 'ticket_close_btn',   '关闭工单', 'ticket:close',   5),
    ('ticket_list', 'ticket_assign_btn',  '分派工单', 'ticket:assign',  6),
    ('ticket_list', 'ticket_delete_btn',  '删除工单', 'ticket:delete',  7),
    ('ticket_list', 'ticket_comment_btn', '回复工单', 'ticket:comment', 8),
    ('ticket_list', 'ticket_note_btn',    '内部备注', 'ticket:note',    9)
) AS v(parent_code, code, name, permission, sort_order)
JOIN menus p ON p.code = v.parent_code
ON CONFLICT (code) DO NOTHING;

-- ⑦-c menu_apis：仅绑定 ticket_list（level 2 页面），覆盖 §3 API 表全部 16 条路由
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m, (VALUES
    -- 工单 CRUD + 协作
    ('ticket_list', '/api/v1/tickets',                 'GET'),
    ('ticket_list', '/api/v1/tickets',                 'POST'),
    ('ticket_list', '/api/v1/tickets/:id',             'GET'),
    ('ticket_list', '/api/v1/tickets/update',           'POST'),
    ('ticket_list', '/api/v1/tickets/close',            'POST'),
    ('ticket_list', '/api/v1/tickets/assign',           'POST'),
    ('ticket_list', '/api/v1/tickets/delete',           'POST'),
    ('ticket_list', '/api/v1/tickets/:id/comments',     'GET'),
    ('ticket_list', '/api/v1/tickets/comments',         'POST'),
    ('ticket_list', '/api/v1/tickets/notes',            'POST'),
    ('ticket_list', '/api/v1/tickets/:id/relations',    'GET'),
    ('ticket_list', '/api/v1/tickets/relations',        'POST'),
    -- 元数据（2a）
    ('ticket_list', '/api/v1/ticket-types',              'GET'),
    ('ticket_list', '/api/v1/ticket-types/:code/fields',  'GET'),
    -- 模板 / 关联（2a 前移）
    ('ticket_list', '/api/v1/ticket-templates',          'GET'),
    ('ticket_list', '/api/v1/ticket-templates/:code',     'GET')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code
ON CONFLICT DO NOTHING;

-- ⑦-d 角色绑定：admin/superadmin 获得本迁移新增的所有菜单（catalog + page + 9 个按钮）
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r, menus m
WHERE r.code IN ('superadmin', 'admin')
  AND m.code IN (
    'ticket_manage', 'ticket_list',
    'ticket_list_btn', 'ticket_create_btn', 'ticket_read_btn',
    'ticket_update_btn', 'ticket_close_btn', 'ticket_assign_btn',
    'ticket_delete_btn', 'ticket_comment_btn', 'ticket_note_btn'
  )
ON CONFLICT DO NOTHING;

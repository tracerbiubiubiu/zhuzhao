-- 000010 companion: 工单管理菜单（catalog/page/button 三层）+ menu_apis + 角色绑定
-- SSOT: docs/phase2/09-ticket.md §1.1  +  D2: ticket:list/create/read/update/close/assign/delete/comment/note
-- 对齐 migrations/000002_seed.up.sql 插入风格：
--   menu_type = SMALLINT (1=目录 2=页面 3=按钮)，parent_id 通过 code JOIN 反查，不硬写序列 ID
--   操作类 POST 路径不含 :id（id 放 body）；角色绑定页面即获该页全部 menu_apis；按钮菜单用于 UI 可见性 + AssignMenus 细粒度授权

-- ========================================================================
-- ① 目录 + 页面菜单（level 1/2）
-- ========================================================================
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

-- ========================================================================
-- ② 按钮菜单（level 3，9 动作 = list/create/read/update/close/assign/delete/comment/note）
--    不写 menu_apis（page 级已统一绑定全部 ticket API）；权限码仅用于：
--      a) 前端按钮显示开关（UI 查询用户菜单树）
--      b) AssignMenus 细粒度授权（将来限制某角色只能看列表不能关单）
-- ========================================================================
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

-- ========================================================================
-- ③ menu_apis：仅绑定 ticket_list（level 2 页面），覆盖 §3 API 表全部路由
--    页面菜单即获该页全部 API；按钮菜单不单独挂 API（Casbin 授权页 = 全 API 放行，细粒度由资源级 canOperate 兜底）
-- ========================================================================
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

-- ========================================================================
-- ④ 角色绑定：admin/superadmin 获得本迁移新增的所有菜单（catalog + page + 9 个按钮）
--    对齐 000002_seed.up.sql 通配模式，ON CONFLICT DO NOTHING 幂等重放
-- ========================================================================
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

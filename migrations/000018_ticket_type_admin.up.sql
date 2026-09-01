-- 000018: 工单类型/字段/模板管理闭环（IW3/BK-18）
-- SSOT: docs/phase2/00 §9 BK-18、docs/phase3/12-frontend
-- ① 字段校验正则（G2：创建时按 schema 校验 required+regex）
ALTER TABLE ticket_type_fields
    ADD COLUMN IF NOT EXISTS validate_regex VARCHAR(200);

-- ② 管理页菜单（ticket_manage 目录下 level 2 页面，permission = ticket:type:manage）
--    L1：admin/superadmin 走 matcher 通配；operator 仅在 AssignMenus 绑定本页后放行
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES ('ticket_type_manage', '类型配置',
        (SELECT id FROM menus WHERE code = 'ticket_manage'), 2,
        '/tickets/types', 'ticket/type/index', 'setting', 'ticket:type:manage', 3, true)
ON CONFLICT DO NOTHING;

-- ③ menu_apis：管理 CRUD 路由挂类型配置页（对齐 000010 ⑦-c 模式）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('/api/v1/ticket-types',              'POST'),
    ('/api/v1/ticket-types/:code',        'PUT'),
    ('/api/v1/ticket-types/:code',        'DELETE'),
    ('/api/v1/ticket-types/:code/fields', 'PUT'),
    ('/api/v1/ticket-templates',          'POST'),
    ('/api/v1/ticket-templates/:code',    'PUT'),
    ('/api/v1/ticket-templates/:code',    'DELETE')
) AS v(api_path, api_method)
WHERE m.code = 'ticket_type_manage'
ON CONFLICT DO NOTHING;

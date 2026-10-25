-- 000073: S3a B3-2 分派规则管理页菜单行（孤岛页补路由）
-- 页面 views/ticket/rules/index.vue（ui 45a72256，defineOptions name=ticket_assignment_rules）已落成，
-- 但无菜单种子 → 无动态路由、无侧栏入口、viewNames 守卫（单向：种子→页面）不报——E2E 补齐时核出。
-- 03 号 A.2：管理面归 B3-2「类型管理页族扩展」，权限沿用 ticket:type:manage（零新码）。
-- icon 复用 000018 的裸名 setting（前端 MENU_ICON_MAP 已登记，零前端改动）。
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES ('ticket_assignment_rules', '分派规则',
        (SELECT id FROM menus WHERE code = 'ticket_manage'), 2,
        '/tickets/rules', 'ticket/rules/index', 'setting', 'ticket:type:manage', 4, true)
ON CONFLICT DO NOTHING;

-- 四端点在 000070 已绑 ticket_type_manage；本页再绑一份，使 AssignMenus 单独分配本页给 operator 时
-- L1（menu_apis 派生）自洽，不必连带分配类型配置页（宁窄勿宽）。NOT EXISTS 幂等。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('/api/v1/assignment-rules',         'GET'),
    ('/api/v1/assignment-rules',         'POST'),
    ('/api/v1/assignment-rules/delete',  'POST'),
    ('/api/v1/assignment-rules/enabled', 'POST')
) AS v(api_path, api_method)
WHERE m.code = 'ticket_assignment_rules' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = v.api_method);

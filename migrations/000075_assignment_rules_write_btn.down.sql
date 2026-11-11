-- 000075 down: 逆拆按钮行并回种「写挂页面行」原状（000070/000073 时代形态）。
-- casbin 不逆向重建：三写行曾由 AssignMenus 按误绑定派生，down 后由管理员经角色管理重新分配按钮行
-- 派生正确策略；admin/superadmin 走 matcher 通配不依赖 casbin 行。
-- 全新往返库中本迁移 up 阶段 casbin DELETE 影响 0 行（无 AssignMenus 跑过）——roundtrip 无漂移。
DELETE FROM role_menus WHERE menu_id = (SELECT id FROM menus WHERE code = 'ticket_assignment_rules_write_btn');
DELETE FROM menu_apis WHERE menu_id = (SELECT id FROM menus WHERE code = 'ticket_assignment_rules_write_btn');
DELETE FROM menus WHERE code = 'ticket_assignment_rules_write_btn';

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('ticket_type_manage', '/api/v1/assignment-rules', 'POST'),
    ('ticket_type_manage', '/api/v1/assignment-rules/delete', 'POST'),
    ('ticket_type_manage', '/api/v1/assignment-rules/enabled', 'POST'),
    ('ticket_assignment_rules', '/api/v1/assignment-rules', 'POST'),
    ('ticket_assignment_rules', '/api/v1/assignment-rules/delete', 'POST'),
    ('ticket_assignment_rules', '/api/v1/assignment-rules/enabled', 'POST')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = v.api_method);

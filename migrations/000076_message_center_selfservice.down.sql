-- 000076 down: 回种 000072/000074 形态——菜单行翻可见、三端点重挂页面行、admin/superadmin 重绑。
-- casbin 不逆向重建（由 AssignMenus 重放派生）；router 侧 selfService 分组为代码态，down 不触及。
UPDATE menus SET visible = true WHERE code = 'message_center' AND deleted_at IS NULL;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('/api/v1/notifications/inbox',    'GET'),
    ('/api/v1/notifications/read',     'POST'),
    ('/api/v1/notifications/read-all', 'POST')
) AS v(api_path, api_method)
WHERE m.code = 'message_center' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = v.api_method);

INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r JOIN menus m ON m.code = 'message_center'
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL AND m.deleted_at IS NULL
ON CONFLICT (role_id, menu_id) DO NOTHING;

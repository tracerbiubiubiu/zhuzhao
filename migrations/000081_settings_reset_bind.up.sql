-- 000081: 设置恢复默认端点绑定（2026-11-03 设置页走查批——业界惯例 Reset to default）。
--
-- POST /api/v1/system/settings/reset 挂 system_setting_reset 按钮行（B 案守卫：页面=读、
-- 写挂按钮——照 000048 惯例三件套：按钮行 + menu_apis + role_menus admin/superadmin）。
-- 语义：删除 system_settings 覆盖行回 config 出厂值（幂等；tier 分层在服务端 catalog 校验）。

INSERT INTO menus (parent_id, menu_type, code, name, path, component, permission, sort_order, icon, visible, is_system)
SELECT m.id, 3, 'system_setting_reset', '恢复默认设置', '', '', 'system:setting:update', 2, '', true, true
FROM menus m WHERE m.code = 'system_setting'
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/system/settings/reset', 'POST' FROM menus m WHERE m.code = 'system_setting_reset'
ON CONFLICT DO NOTHING;

INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r CROSS JOIN menus m
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL
  AND m.code = 'system_setting_reset'
ON CONFLICT DO NOTHING;

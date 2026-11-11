-- 000048 down: 功能绑定逆拆——menu_apis / role_menus / 表。
-- 菜单行（system_setting/system_setting_update）留档不删：自增 ID 在 down→up 间漂移
-- 会触往返门禁 parent_id 快照差异（对齐 000036 惯例——无绑定的菜单=惰性 UI 条目，
-- 不构成权限面；生产如需彻底清理可手工 DELETE FROM menus WHERE code LIKE 'system_setting%'）。
DELETE FROM menu_apis WHERE (api_path, api_method) IN ((
    '/api/v1/system/settings', 'GET'), (
    '/api/v1/system/settings/update', 'POST'));

DELETE FROM role_menus WHERE menu_id IN (
    SELECT id FROM menus WHERE code IN ('system_setting', 'system_setting_update'));

DROP TABLE IF EXISTS system_settings;

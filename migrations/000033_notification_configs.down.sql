-- 还原（审计修复 2026-09-30：原文件为空——down 非精确逆，菜单/绑定/表 down 后全残留）
-- role_menus（000034 补绑段引用 90001/90002——先删绑定）
DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('system_notification', 'notification_write_btn'));

-- menu_apis（复合条件精确删）
DELETE FROM menu_apis WHERE (api_path, api_method) IN ((
    '/api/v1/notifications', 'GET'), (
    '/api/v1/notifications', 'POST'), (
    '/api/v1/notifications/update', 'POST'), (
    '/api/v1/notifications/delete', 'POST'));

-- 菜单（000033 显式高位 id 90001/90002）
DELETE FROM menus WHERE id IN (90001, 90002);

-- 表
DROP TABLE IF EXISTS notification_configs;

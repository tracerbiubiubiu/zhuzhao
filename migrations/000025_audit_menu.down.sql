-- 回滚：审计日志权限面
-- W1 补（全链 down 验证抓漏）：role_menus 先清
DELETE FROM role_menus
WHERE menu_id IN (SELECT id FROM menus WHERE code = 'audit_log');

DELETE FROM menu_apis
WHERE menu_id IN (SELECT id FROM menus WHERE code = 'audit_log');
DELETE FROM menus WHERE code = 'audit_log';

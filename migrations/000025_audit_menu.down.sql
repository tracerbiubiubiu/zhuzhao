-- 回滚：审计日志权限面
DELETE FROM menu_apis
WHERE menu_id IN (SELECT id FROM menus WHERE code = 'audit_log');
DELETE FROM menus WHERE code = 'audit_log';

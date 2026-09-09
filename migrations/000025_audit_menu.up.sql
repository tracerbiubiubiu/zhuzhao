-- BK-22 对账发现（2026-09-09）：GET /api/v1/audit/logs（biz 组 CasbinAuth 强制）
-- 无权限面注册——仅 admin/superadmin 通配可达。补「审计日志」菜单 + audit:read 码
-- + menu_apis 绑定（AssignMenus 运营分配后非管理员可授审计只读）。
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES ('audit_log', '审计日志', NULL, 2, '/audit', 'audit/log/index', 'audit-log', 'audit:read', 5, true)
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/audit/logs', 'GET' FROM menus m WHERE m.code = 'audit_log'
ON CONFLICT DO NOTHING;

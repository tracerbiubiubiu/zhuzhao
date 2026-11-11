-- 000074: phase5 三页面 role_menus 回补（000031/000034/000048 惯例——新菜单页须绑 admin+superadmin）
-- 000067 intel_query / 000072 message_center 建菜单行时漏绑（000073 ticket_assignment_rules 同病同批）：
-- GetUserMenus 按 role_menus INNER JOIN 无通配旁路 → 页面全员不可达（侧栏无入口、动态路由不注册、
-- 铃铛 goInbox 跳 /messages 404）。E2E 补齐批（2026-10-25）核出。
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id
FROM roles r JOIN menus m ON m.code IN ('intel_query', 'message_center', 'ticket_assignment_rules')
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL
  AND m.deleted_at IS NULL
ON CONFLICT (role_id, menu_id) DO NOTHING;

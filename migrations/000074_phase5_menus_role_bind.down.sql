-- 000074 down: 解绑三页面（菜单行归各自迁移管理）。role_menus 先清——
-- 下行序（000074→000073→000072→000067）中 000072/000067 的 down 直接删菜单行，残留绑定会触发 FK。
DELETE FROM role_menus
WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('intel_query', 'message_center', 'ticket_assignment_rules'));

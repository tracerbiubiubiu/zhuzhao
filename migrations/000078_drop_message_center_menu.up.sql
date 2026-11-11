-- 000078: 删除 message_center 菜单行（G-23 收尾——000076 惰性化留下的"幽灵项"）
-- 000076 为避自增 ID 漂移只把行 visible=false，但管理端 GetTree 含隐藏节点 → 「角色管理→分配菜单」树里
-- 该行可见可勾、勾了无任何效果（无 apis、路由已是常量路由）。复核往返门禁语义：行在两次全量 up 的终态快照中
-- 均不存在，删行同样零漂移（000073 删叶子行先例）——故补删。inbox:read 码随行退役（唯一使用点；三端点已 selfService）。
-- role_menus/menu_apis 000076 已清，此处幂等再清防 FK。
DELETE FROM role_menus WHERE menu_id = (SELECT id FROM menus WHERE code = 'message_center');
DELETE FROM menu_apis  WHERE menu_id = (SELECT id FROM menus WHERE code = 'message_center');
DELETE FROM menus WHERE code = 'message_center';

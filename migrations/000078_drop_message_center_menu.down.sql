-- 000078 down: 回种 000076 终态（行存在、visible=false、零绑定）——下行序中 000076 down 随后翻可见+重挂端点+重绑角色，
-- 000072 down 最终删行。列值照抄 000072 原种子，仅 visible 取 000076 终态。
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system, visible)
VALUES ('message_center', '消息中心', NULL, 2, '/messages', 'message/center/index', 'message', 'inbox:read', 7, true, false)
ON CONFLICT DO NOTHING;

-- 000010 down: 工单模块（表 + 菜单 + menu_apis + 角色绑定）
-- 清理顺序（防 FK 约束）：role_menus → menu_apis → menus（按钮 → 页面 → 目录）→ ticket 表（FK 反序）

-- ⑦ 菜单相关清理（menus/roles 表本身不在本迁移创建，只清本迁移写入的行）
DELETE FROM role_menus
WHERE menu_id IN (SELECT id FROM menus WHERE code IN (
    'ticket_manage', 'ticket_list',
    'ticket_list_btn', 'ticket_create_btn', 'ticket_read_btn',
    'ticket_update_btn', 'ticket_close_btn', 'ticket_assign_btn',
    'ticket_delete_btn', 'ticket_comment_btn', 'ticket_note_btn'
));

DELETE FROM menu_apis
WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('ticket_manage', 'ticket_list'));

DELETE FROM menus WHERE code IN (
    -- level 3 按钮（先删子节点）
    'ticket_list_btn', 'ticket_create_btn', 'ticket_read_btn',
    'ticket_update_btn', 'ticket_close_btn', 'ticket_assign_btn',
    'ticket_delete_btn', 'ticket_comment_btn', 'ticket_note_btn',
    -- level 2 页面
    'ticket_list',
    -- level 1 目录（最后删父节点）
    'ticket_manage'
);

-- ①~⑤ 表 DROP（FK 反序：events → comments → tickets → type_fields → types）
DROP TABLE IF EXISTS ticket_events;
DROP TABLE IF EXISTS ticket_comments;
DROP TABLE IF EXISTS tickets;
DROP TABLE IF EXISTS ticket_type_fields;
DROP TABLE IF EXISTS ticket_types;

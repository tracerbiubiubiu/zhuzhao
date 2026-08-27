-- 000010 companion down: 工单管理菜单 + menu_apis + 角色绑定（按依赖反序清理）
-- 清理顺序（防 FK 约束）：role_menus → menu_apis → menus（先子后父：按钮 → 页面 → 目录）

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

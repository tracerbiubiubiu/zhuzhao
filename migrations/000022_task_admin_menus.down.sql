-- 000022 down: 任务管理菜单（W0a-P0-4 修复——原版只删 task_center/task_manage 两行，
-- 不删 3 个按钮行：按钮 parent_id FK NO ACTION，且 AssignMenus 绑定过按钮时
-- role_menus 残留 → down 必报 23503，22 及以下回滚链整体卡死，违反 up/down
-- 成对铁律。照 000010 down 正确范式：role_menus → menu_apis → 按钮 → 页面/目录。

-- 角色绑定（运营 AssignMenus 分配过的按钮/页面一并清理）
DELETE FROM role_menus
WHERE menu_id IN (SELECT id FROM menus WHERE code IN (
    'task_manage', 'task_center',
    'task_submit_btn', 'task_read_btn', 'task_manage_btn'
));

-- 路由绑定
DELETE FROM menu_apis
WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('task_center'));

-- 按钮（先删子节点，防 parent FK）
DELETE FROM menus WHERE code IN ('task_submit_btn', 'task_read_btn', 'task_manage_btn');

-- 页面与目录
DELETE FROM menus WHERE code IN ('task_center', 'task_manage');

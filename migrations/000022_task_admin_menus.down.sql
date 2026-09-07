DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('task_center','task_manage'));
DELETE FROM menu_apis WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('task_center','task_manage'));
DELETE FROM menus WHERE code IN ('task_center','task_manage');

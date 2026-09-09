-- 回滚：activelist 权限面（菜单/按钮/路由绑定，含 AssignMenus 可能产生的
-- casbin 行由 AssignMenus 重建链路自然消亡——此处只清目录结构）。
DELETE FROM menu_apis
WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('al_types', 'al_data'));
DELETE FROM menus WHERE code IN ('al_type_manage_btn', 'al_data_write_btn');
DELETE FROM menus WHERE code IN ('al_data', 'al_types', 'al_manage');

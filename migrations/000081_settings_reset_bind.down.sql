-- 000081 down: 恢复默认端点绑定退役（按钮行/menu_apis/role_menus 三件逆向）
DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE code = 'system_setting_reset');
DELETE FROM menu_apis WHERE menu_id IN (SELECT id FROM menus WHERE code = 'system_setting_reset');
DELETE FROM menus WHERE code = 'system_setting_reset';

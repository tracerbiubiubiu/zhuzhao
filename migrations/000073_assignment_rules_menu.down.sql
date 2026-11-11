-- 000073 down: 逆序 role_menus → menu_apis → 菜单行（000018 down 同款——分配过即触发 role_menus FK）。
-- 只删本页 menu_id 维度的绑定；000070 挂在 ticket_type_manage 上的同路径绑定不动。
DELETE FROM role_menus
WHERE menu_id = (SELECT id FROM menus WHERE code = 'ticket_assignment_rules');

DELETE FROM menu_apis
WHERE menu_id = (SELECT id FROM menus WHERE code = 'ticket_assignment_rules');

DELETE FROM menus WHERE code = 'ticket_assignment_rules';

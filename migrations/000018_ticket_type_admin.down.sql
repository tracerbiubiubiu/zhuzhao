-- 000018 down: 移除类型管理闭环（逆序：menu_apis → 菜单 → validate_regex 列）

-- W1 补（全链 down 验证抓漏）：role_menus 先清（运营 AssignMenus 分配过即触发 FK 23503）
DELETE FROM role_menus
WHERE menu_id = (SELECT id FROM menus WHERE code = 'ticket_type_manage');

DELETE FROM menu_apis
WHERE menu_id = (SELECT id FROM menus WHERE code = 'ticket_type_manage');

DELETE FROM menus WHERE code = 'ticket_type_manage';

ALTER TABLE ticket_type_fields
    DROP COLUMN IF EXISTS validate_regex;

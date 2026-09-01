-- 000018 down: 移除类型管理闭环（逆序：menu_apis → 菜单 → validate_regex 列）

DELETE FROM menu_apis
WHERE menu_id = (SELECT id FROM menus WHERE code = 'ticket_type_manage');

DELETE FROM menus WHERE code = 'ticket_type_manage';

ALTER TABLE ticket_type_fields
    DROP COLUMN IF EXISTS validate_regex;

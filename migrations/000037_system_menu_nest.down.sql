-- 还原（精确逆：四页 parent 复位顶层）
UPDATE menus SET parent_id = NULL
WHERE code IN ('system_user', 'system_role', 'system_menu', 'system_org')
  AND parent_id = (SELECT id FROM menus WHERE code = 'system');

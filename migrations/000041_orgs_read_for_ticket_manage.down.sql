-- 000041 回滚：收回业务角色的组织树读权限
DELETE FROM menu_apis
WHERE api_path = '/api/v1/orgs' AND api_method = 'GET'
  AND menu_id = (SELECT id FROM menus WHERE code = 'ticket_manage' AND menu_type = 1);

DELETE FROM casbin_rule
WHERE ptype = 'p' AND v1 = '/api/v1/orgs' AND v2 = 'GET'
  AND v0 IN (
    SELECT 'role::' || r.code FROM roles r
    JOIN role_menus rm ON rm.role_id = r.id
    JOIN menus m ON m.id = rm.menu_id
    WHERE m.code = 'ticket_manage' AND m.menu_type = 1
  );

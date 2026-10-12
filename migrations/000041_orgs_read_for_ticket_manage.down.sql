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
      -- 忠实逆守卫：该角色若还有**其他菜单**也授予 GET /orgs（如同时绑 system_org，
      -- 其规则源于彼处），则不删——否则回滚即静默丢权限（2026-10-12 复核批 A/B 实证）
      AND NOT EXISTS (
        SELECT 1 FROM role_menus rm2
        JOIN menu_apis ma2 ON ma2.menu_id = rm2.menu_id
        WHERE rm2.role_id = r.id
          AND ma2.api_path = '/api/v1/orgs' AND ma2.api_method = 'GET'
          AND rm2.menu_id <> m.id
      )
  );

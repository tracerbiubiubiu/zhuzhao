-- 000042 回滚：仅收回「绑了工单管理子菜单但未绑目录本体」角色的 GET /orgs 规则
--（000041 已授予的目录绑定角色不动，保持忠实逆）
DELETE FROM casbin_rule c
WHERE c.ptype = 'p' AND c.v1 = '/api/v1/orgs' AND c.v2 = 'GET'
  AND c.v0 IN (
    SELECT DISTINCT 'role::' || r.code
    FROM roles r
    JOIN role_menus rm ON rm.role_id = r.id
    JOIN menus m ON m.id = rm.menu_id
    WHERE m.parent_id = (SELECT id FROM menus WHERE code = 'ticket_manage' AND menu_type = 1)
      AND NOT EXISTS (
        SELECT 1 FROM role_menus rm2
        JOIN menus m2 ON m2.id = rm2.menu_id
        WHERE rm2.role_id = r.id AND m2.code = 'ticket_manage'
      )
  );

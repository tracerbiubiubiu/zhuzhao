-- 000042 回滚：先删 up 补授的 menu_apis 行，再按「来源维度」收回 casbin 规则。
-- 顺序承重（2026-10-12 复核批）：若后删 menu_apis，下方守卫会把自家补授行当
-- 「其他来源」豁免，casbin 规则永远删不掉——必须先删来源再判守卫。
-- casbin 守卫与 000041 down 同款忠实逆：角色若还有其他菜单（如 system_org）的
-- menu_apis 也授予 GET /orgs，其规则源于彼处，不删——否则回滚即静默丢权限
--（首版按「未绑目录本体」维度判断，双绑角色被误删，同类缺陷复现已修）。
DELETE FROM menu_apis
WHERE api_path = '/api/v1/orgs' AND api_method = 'GET'
  AND menu_id IN (
    SELECT id FROM menus
    WHERE code IN ('ticket_type_manage', 'ticket_list') AND deleted_at IS NULL
  );

DELETE FROM casbin_rule c
WHERE c.ptype = 'p' AND c.v1 = '/api/v1/orgs' AND c.v2 = 'GET'
  AND c.v0 IN (
    SELECT DISTINCT 'role::' || r.code
    FROM roles r
    JOIN role_menus rm ON rm.role_id = r.id
    JOIN menus m ON m.id = rm.menu_id
    WHERE m.parent_id = (SELECT id FROM menus WHERE code = 'ticket_manage' AND menu_type = 1)
      AND m.deleted_at IS NULL
      AND NOT EXISTS (
        SELECT 1 FROM role_menus rm2
        JOIN menu_apis ma2 ON ma2.menu_id = rm2.menu_id
        WHERE rm2.role_id = r.id
          AND ma2.api_path = '/api/v1/orgs' AND ma2.api_method = 'GET'
      )
  );

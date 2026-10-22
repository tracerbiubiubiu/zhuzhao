-- 000052 回滚：先删来源（menu_apis）再按来源维度判守卫收回 casbin（000042 顺序
-- 承重同款——后删来源会让守卫把自家补授行当「其他来源」豁免）；路由归 SelfService
-- 与 catalogExempt 登记须随代码同批回退（本迁移只管权限面）。
DELETE FROM menu_apis
WHERE api_path = '/api/v1/ticket-types/:code/fields/impact' AND api_method = 'GET'
  AND menu_id IN (
    SELECT id FROM menus
    WHERE code = 'ticket_type_manage' AND deleted_at IS NULL
  );

DELETE FROM casbin_rule c
WHERE c.ptype = 'p' AND c.v1 = '/api/v1/ticket-types/:code/fields/impact' AND c.v2 = 'GET'
  AND c.v0 IN (
    SELECT DISTINCT 'role::' || r.code
    FROM roles r
    JOIN role_menus rm ON rm.role_id = r.id
    JOIN menus m ON m.id = rm.menu_id
    WHERE m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
      AND NOT EXISTS (
        SELECT 1 FROM role_menus rm2
        JOIN menu_apis ma2 ON ma2.menu_id = rm2.menu_id
        WHERE rm2.role_id = r.id
          AND ma2.api_path = '/api/v1/ticket-types/:code/fields/impact'
          AND ma2.api_method = 'GET'
      )
  );

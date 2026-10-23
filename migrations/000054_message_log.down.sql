-- 000054 回滚：menu_apis 来源先删→casbin 守卫判源收回→规则/日志表删
-- （000042 顺序承重同款：后删来源会让守卫把自家补授行当「其他来源」豁免）
DELETE FROM menu_apis WHERE api_path = '/api/v1/message-templates' AND api_method = 'GET'
  AND menu_id IN (SELECT id FROM menus WHERE code = 'ticket_type_manage' AND deleted_at IS NULL);

DELETE FROM casbin_rule c
WHERE c.ptype='p' AND c.v1='/api/v1/message-templates' AND c.v2='GET'
  AND c.v0 IN (
    SELECT DISTINCT 'role::' || r.code FROM roles r
    JOIN role_menus rm ON rm.role_id = r.id
    JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
    AND NOT EXISTS (
      SELECT 1 FROM role_menus rm2 JOIN menu_apis ma2 ON ma2.menu_id = rm2.menu_id
      WHERE rm2.role_id = r.id AND ma2.api_path = '/api/v1/message-templates' AND ma2.api_method = 'GET'
    )
  );

DROP TABLE IF EXISTS message_rules;
DROP TABLE IF EXISTS message_log;

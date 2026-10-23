-- 000055 回滚：先删 menu_apis 来源→casbin 守卫判源收回（000042 顺序承重）
DELETE FROM menu_apis
WHERE (api_path, api_method) IN (
    ('/api/v1/message-logs', 'GET'),
    ('/api/v1/message-rules', 'GET'),
    ('/api/v1/message-rules', 'POST'),
    ('/api/v1/message-rules/delete', 'POST')
)
AND menu_id IN (SELECT id FROM menus WHERE code = 'ticket_type_manage' AND deleted_at IS NULL);

DELETE FROM casbin_rule c
WHERE c.ptype = 'p'
AND (c.v1, c.v2) IN (
    ('/api/v1/message-logs', 'GET'),
    ('/api/v1/message-rules', 'GET'),
    ('/api/v1/message-rules', 'POST'),
    ('/api/v1/message-rules/delete', 'POST')
)
AND c.v0 IN (
    SELECT DISTINCT 'role::' || r.code FROM roles r
    JOIN role_menus rm ON rm.role_id = r.id
    JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
    WHERE NOT EXISTS (
        SELECT 1 FROM role_menus rm2 JOIN menu_apis ma2 ON ma2.menu_id = rm2.menu_id
        WHERE rm2.role_id = r.id AND (ma2.api_path, ma2.api_method) IN (
            ('/api/v1/message-logs', 'GET'),
            ('/api/v1/message-rules', 'GET'),
            ('/api/v1/message-rules', 'POST'),
            ('/api/v1/message-rules/delete', 'POST')
        )
    )
);

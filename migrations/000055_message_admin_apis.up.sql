-- 000055: S6 B6-6 消息管理四端点 menu_apis 正式绑定（撤 catalogExempt 误挂）
-- （000052 同款模板：先补 menu_apis 来源行→再对存量绑定直插 casbin）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.path, v.method
FROM menus m,
     (VALUES
        ('/api/v1/message-logs',          'GET'),
        ('/api/v1/message-rules',         'GET'),
        ('/api/v1/message-rules',         'POST'),
        ('/api/v1/message-rules/delete',  'POST')
     ) AS v(path, method)
WHERE m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
AND NOT EXISTS (
    SELECT 1 FROM menu_apis ma
    WHERE ma.menu_id = m.id AND ma.api_path = v.path AND ma.api_method = v.method
);

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, v.path, v.method
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
CROSS JOIN (VALUES
    ('/api/v1/message-logs',          'GET'),
    ('/api/v1/message-rules',         'GET'),
    ('/api/v1/message-rules',         'POST'),
    ('/api/v1/message-rules/delete',  'POST')
) AS v(path, method)
WHERE NOT EXISTS (
    SELECT 1 FROM casbin_rule c
    WHERE c.ptype = 'p' AND c.v0 = 'role::' || r.code AND c.v1 = v.path AND c.v2 = v.method
);

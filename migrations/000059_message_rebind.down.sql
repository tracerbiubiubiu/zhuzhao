-- 000059 回滚：消息端点绑回 ticket_type_manage + system_notification visible=false
DELETE FROM menu_apis
WHERE menu_id IN (SELECT id FROM menus WHERE code = 'system_notification' AND deleted_at IS NULL)
AND (api_path, api_method) IN (
    ('/api/v1/message-templates', 'GET'),
    ('/api/v1/message-templates', 'POST'),
    ('/api/v1/message-templates/update', 'POST'),
    ('/api/v1/message-templates/delete', 'POST'),
    ('/api/v1/message-logs', 'GET'),
    ('/api/v1/message-rules', 'GET'),
    ('/api/v1/message-rules', 'POST'),
    ('/api/v1/message-rules/delete', 'POST')
);

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.path, v.method
FROM menus m,
     (VALUES
        ('/api/v1/message-templates', 'GET'),
        ('/api/v1/message-templates', 'POST'),
        ('/api/v1/message-templates/update', 'POST'),
        ('/api/v1/message-templates/delete', 'POST'),
        ('/api/v1/message-logs', 'GET'),
        ('/api/v1/message-rules', 'GET'),
        ('/api/v1/message-rules', 'POST'),
        ('/api/v1/message-rules/delete', 'POST')
     ) AS v(path, method)
WHERE m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
AND NOT EXISTS (
    SELECT 1 FROM menu_apis ma
    WHERE ma.menu_id = m.id AND ma.api_path = v.path AND ma.api_method = v.method
);

DELETE FROM casbin_rule WHERE ptype='p' AND v1 LIKE '/api/v1/message-%';
UPDATE menus SET visible = false WHERE code = 'system_notification' AND deleted_at IS NULL;

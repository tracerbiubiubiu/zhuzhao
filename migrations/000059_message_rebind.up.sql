-- S6 外审 F-2-4 修复：消息管理 12 端点从 ticket_type_manage 重绑到 system_notification
-- （权限语义修正：消息模板/规则/发送记录属通知中心，非工单类型管理面）
-- 先从 ticket_type_manage 删旧绑定
DELETE FROM menu_apis
WHERE menu_id IN (SELECT id FROM menus WHERE code = 'ticket_type_manage' AND deleted_at IS NULL)
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

-- 绑到 system_notification（000033 预登记菜单——此前 visible=false 无视图）
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
WHERE m.code = 'system_notification' AND m.deleted_at IS NULL
AND NOT EXISTS (
    SELECT 1 FROM menu_apis ma
    WHERE ma.menu_id = m.id AND ma.api_path = v.path AND ma.api_method = v.method
);

-- 存量 casbin：删旧角色的消息端点权限，补 system_notification 绑定角色的权限
DELETE FROM casbin_rule
WHERE ptype = 'p'
AND (v1, v2) IN (
    ('/api/v1/message-templates', 'GET'),
    ('/api/v1/message-templates', 'POST'),
    ('/api/v1/message-templates/update', 'POST'),
    ('/api/v1/message-templates/delete', 'POST'),
    ('/api/v1/message-logs', 'GET'),
    ('/api/v1/message-rules', 'GET'),
    ('/api/v1/message-rules', 'POST'),
    ('/api/v1/message-rules/delete', 'POST')
);

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, v.path, v.method
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.code = 'system_notification' AND m.deleted_at IS NULL
CROSS JOIN (VALUES
    ('/api/v1/message-templates', 'GET'),
    ('/api/v1/message-templates', 'POST'),
    ('/api/v1/message-templates/update', 'POST'),
    ('/api/v1/message-templates/delete', 'POST'),
    ('/api/v1/message-logs', 'GET'),
    ('/api/v1/message-rules', 'GET'),
    ('/api/v1/message-rules', 'POST'),
    ('/api/v1/message-rules/delete', 'POST')
) AS v(path, method)
WHERE NOT EXISTS (
    SELECT 1 FROM casbin_rule c
    WHERE c.ptype = 'p' AND c.v0 = 'role::' || r.code AND c.v1 = v.path AND c.v2 = v.method
);

-- 菜单 visible=true（有视图了——S6 UI 管理页）
UPDATE menus SET visible = true WHERE code = 'system_notification' AND deleted_at IS NULL;

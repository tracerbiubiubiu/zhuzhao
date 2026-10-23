-- 000059 回滚（外审 F3-1-8 修正——原版 DELETE casbin 无重建=不可逆失权）：
-- 三段式 = 撤来源 → provenance 判源 → 按 ticket_type_manage 绑定角色重建执行面
-- ① 撤 system_notification 侧来源
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

-- ② 按 provenance 判源收回 casbin（角色无其他来源时才删）
DELETE FROM casbin_rule c
WHERE c.ptype = 'p'
AND (c.v1, c.v2) IN (
    ('/api/v1/message-templates', 'GET'),
    ('/api/v1/message-templates', 'POST'),
    ('/api/v1/message-templates/update', 'POST'),
    ('/api/v1/message-templates/delete', 'POST'),
    ('/api/v1/message-logs', 'GET'),
    ('/api/v1/message-rules', 'GET'),
    ('/api/v1/message-rules', 'POST'),
    ('/api/v1/message-rules/delete', 'POST')
)
AND c.v0 IN (
    SELECT DISTINCT 'role::' || r.code FROM roles r
    JOIN role_menus rm ON rm.role_id = r.id
    JOIN menus m ON m.id = rm.menu_id AND m.code = 'system_notification' AND m.deleted_at IS NULL
    WHERE NOT EXISTS (
        SELECT 1 FROM role_menus rm2 JOIN menu_apis ma2 ON ma2.menu_id = rm2.menu_id
        WHERE rm2.role_id = r.id
        AND (ma2.api_path, ma2.api_method) IN (
            ('/api/v1/message-templates', 'GET'),
            ('/api/v1/message-templates', 'POST'),
            ('/api/v1/message-templates/update', 'POST'),
            ('/api/v1/message-templates/delete', 'POST'),
            ('/api/v1/message-logs', 'GET'),
            ('/api/v1/message-rules', 'GET'),
            ('/api/v1/message-rules', 'POST'),
            ('/api/v1/message-rules/delete', 'POST')
        )
    )
);

-- ③ 回绑 ticket_type_manage（menu_apis 来源 + casbin 执行面重建）
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

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, v.path, v.method
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_type_manage' AND m.deleted_at IS NULL
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

-- ④ 菜单 visible 复原
UPDATE menus SET visible = false WHERE code = 'system_notification' AND deleted_at IS NULL;

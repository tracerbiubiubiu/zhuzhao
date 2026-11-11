-- S3a B3-2 分派规则 CRUD 端点（03 号 A.2 管理面——权限沿用类型管理权限）。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('ticket_type_manage', '/api/v1/assignment-rules', 'GET'),
    ('ticket_type_manage', '/api/v1/assignment-rules', 'POST'),
    ('ticket_type_manage', '/api/v1/assignment-rules/delete', 'POST'),
    ('ticket_type_manage', '/api/v1/assignment-rules/enabled', 'POST')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = v.api_method);

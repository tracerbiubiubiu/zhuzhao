-- S3a B3-1 统一转换端点（03 号附录 A.1）：两路由绑 ticket_list 页面行
-- （与既有 /tickets/close 等同受众——L3 属主面+condition 细粒度服务端判）。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('ticket_list', '/api/v1/tickets/transition', 'POST'),
    ('ticket_list', '/api/v1/tickets/:id/transitions/available', 'GET')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = v.api_method);

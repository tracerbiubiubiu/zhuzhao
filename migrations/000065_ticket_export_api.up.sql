-- S10 B10-5 工单导出（10 号 §3.5/A.6）：GET /tickets/export 绑 ticket_list 页面行
-- （查询类 GET 与列表同码同页面——导出不越查询可见性[BK-21]，与 /tickets GET 同受众）。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/tickets/export', 'GET'
FROM menus m
WHERE m.code = 'ticket_list' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = '/api/v1/tickets/export' AND ma.api_method = 'GET');

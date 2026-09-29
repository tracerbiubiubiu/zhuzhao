-- 还原（路由回 biz 组+绑定回页面行）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/dicts/:code/items', 'GET' FROM menus m WHERE m.code = 'system_dict'
ON CONFLICT DO NOTHING;

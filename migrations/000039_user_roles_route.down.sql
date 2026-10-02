-- 000039 精确逆：仅删本迁移新增的绑定行（按 menu×path×method 三元组定位）
DELETE FROM menu_apis
WHERE api_path = '/api/v1/users/:id/roles' AND api_method = 'GET'
  AND menu_id = (SELECT id FROM menus WHERE code = 'system_user' AND deleted_at IS NULL);

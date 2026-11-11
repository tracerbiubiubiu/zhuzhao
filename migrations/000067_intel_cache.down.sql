-- 000067 down：menu_apis 来源行 → 菜单行 → 表（对称回滚）。
DELETE FROM menu_apis WHERE api_path = '/api/v1/intel/query' AND api_method = 'GET';
DELETE FROM menus WHERE code = 'intel_query' AND deleted_at IS NULL;
DROP TABLE IF EXISTS intel_cache;

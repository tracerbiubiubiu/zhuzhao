-- 还原 activelist 三端点参数段 path（复合条件定位，精确逆操作）。
UPDATE menu_apis SET api_path = '/al/api/v1/admin/types/:typeName/deprecate'
WHERE api_path = '/al/api/v1/admin/types/deprecate' AND api_method = 'POST';

UPDATE menu_apis SET api_path = '/al/api/v1/admin/types/:typeName/schema'
WHERE api_path = '/al/api/v1/admin/types/schema' AND api_method = 'POST';

UPDATE menu_apis SET api_path = '/al/api/v1/data/:typeName/:id/restore'
WHERE api_path = '/al/api/v1/data/restore' AND api_method = 'POST';

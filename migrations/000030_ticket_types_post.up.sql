-- W1（BK-18 整改，P4-W1）：工单类型/模板管理 5 端点 PUT/DELETE → POST zhuzhao 风格
-- （code 入 body；standards §3-2「POST URL 不携带业务信息」；§3-5 存量豁免条目随之消亡）。
-- menu_apis 双同步：api_path + api_method 一起改——只改 method 会 dead_binding 拒启。
-- ⚠ 同 path 跨 method 红线：/api/v1/ticket-types/:code 与 /ticket-templates/:code 在
-- ticket_list 页面行另有 GET 绑定（000010:143-147，operator 元数据只读）——UPDATE 一律
-- (api_path, api_method) 复合条件定位，漏 method 会把 operator 的 GET 改成写权限
-- （静默提权，AuditRouteCatalog 不报错——二十四批红线）。

UPDATE menu_apis SET api_path = '/api/v1/ticket-types/update', api_method = 'POST'
WHERE api_path = '/api/v1/ticket-types/:code' AND api_method = 'PUT';

UPDATE menu_apis SET api_path = '/api/v1/ticket-types/delete', api_method = 'POST'
WHERE api_path = '/api/v1/ticket-types/:code' AND api_method = 'DELETE';

UPDATE menu_apis SET api_path = '/api/v1/ticket-types/fields/replace', api_method = 'POST'
WHERE api_path = '/api/v1/ticket-types/:code/fields' AND api_method = 'PUT';

UPDATE menu_apis SET api_path = '/api/v1/ticket-templates/update', api_method = 'POST'
WHERE api_path = '/api/v1/ticket-templates/:code' AND api_method = 'PUT';

UPDATE menu_apis SET api_path = '/api/v1/ticket-templates/delete', api_method = 'POST'
WHERE api_path = '/api/v1/ticket-templates/:code' AND api_method = 'DELETE';

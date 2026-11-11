-- S4 B4-1 动作清单只读端点（04 号 A.1）：GET /jobs/actions 绑 task_center 页面行
-- （task:read 码受众——operator 持提交选单口径可读；与 /jobs 数据面 admin 专属
-- 语义分离——清单=「能提交什么」，menu_apis 与 /tasks GET 同页面行）。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, '/api/v1/jobs/actions', 'GET'
FROM menus m
WHERE m.code = 'task_center' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = '/api/v1/jobs/actions' AND ma.api_method = 'GET');

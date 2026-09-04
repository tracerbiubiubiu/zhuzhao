-- E-④ 任务管理菜单 + 权限码（16 号 §3；menu_apis 驱动 AssignMenus 运营配置；
-- admin/superadmin 已有通配 casbin 策略（000002），新路由即时可用）。
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES
    ('task_manage', '任务管理', NULL, 1, '/tasks', '', 'task', NULL, 3, true),
    ('task_center', '任务中心', NULL, 2, '/tasks', 'task/list/index', 'task-list', 'task:read', 1, true)
ON CONFLICT DO NOTHING;

UPDATE menus SET parent_id = (SELECT id FROM menus WHERE code = 'task_manage')
WHERE code = 'task_center' AND parent_id IS NULL;

INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, v.permission, v.sort_order, true
FROM (VALUES
    ('task_center', 'task_submit_btn', '提交任务',   'task:submit', 1),
    ('task_center', 'task_read_btn',   '查询任务',   'task:read',   2),
    ('task_center', 'task_manage_btn', '任务定义管理', 'task:manage', 3)
) AS v(parent_code, code, name, permission, sort_order)
JOIN menus p ON p.code = v.parent_code
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m JOIN (VALUES
    ('task_center', '/api/v1/tasks',          'POST'),
    ('task_center', '/api/v1/tasks/:id',      'GET'),
    ('task_center', '/api/v1/tasks/cancel',   'POST'),
    ('task_center', '/api/v1/tasks/retry',    'POST'),
    ('task_center', '/api/v1/runs',           'GET'),
    ('task_center', '/api/v1/dead-letters',   'GET'),
    ('task_center', '/api/v1/jobs',           'GET'),
    ('task_center', '/api/v1/jobs',           'POST'),
    ('task_center', '/api/v1/jobs/update',    'POST'),
    ('task_center', '/api/v1/jobs/trigger',   'POST')
) AS v(menu_code, api_path, api_method) ON m.code = v.menu_code
ON CONFLICT DO NOTHING;

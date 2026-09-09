-- 批次 B/E13：activelist 网关路由权限面（menu_apis 驱动 AssignMenus 运营配置）。
-- 反代路由（前缀 /al）照常过 CasbinAuth（§25.1），keyMatch2 匹配 :param 模式——
-- route 清单须与 activelist internal/handler/server.go 保持同步（BK-22 对账将自动化）。
-- admin/superadmin 已有通配 casbin 策略（000002），新路由即时可用。
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES
    ('al_manage', '名单管理', NULL, 1, '/al',      '',               'al',      NULL,                   4, true),
    ('al_data',   '名单数据', NULL, 2, '/al/data', 'al/data/index',  'al-data', 'activelist:data:read', 1, true),
    ('al_types',  '名单类型', NULL, 2, '/al/types','al/types/index', 'al-types','activelist:type:read', 2, true)
ON CONFLICT DO NOTHING;

UPDATE menus SET parent_id = (SELECT id FROM menus WHERE code = 'al_manage')
WHERE code IN ('al_data', 'al_types') AND parent_id IS NULL;

-- 按钮码（前端显隐；路由级授权由 menu_apis → casbin 承担）
INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, v.permission, v.sort_order, true
FROM (VALUES
    ('al_types', 'al_type_manage_btn', '类型管理', 'activelist:type:manage', 1),
    ('al_data',  'al_data_write_btn',  '数据写入', 'activelist:data:write',  2)
) AS v(parent_code, code, name, permission, sort_order)
JOIN menus p ON p.code = v.parent_code
ON CONFLICT DO NOTHING;

-- 网关侧路由清单（/al 前缀 = 网关反代暴露；上游 = activelist /api/v1）。
-- 类型管理（读写）→ al_types；数据 CRUD/导入导出（读写）→ al_data。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m JOIN (VALUES
    ('al_types', '/al/api/v1/admin/types',                    'GET'),
    ('al_types', '/al/api/v1/admin/types/:typeName',          'GET'),
    ('al_types', '/al/api/v1/admin/types/:typeName/history',  'GET'),
    ('al_types', '/al/api/v1/admin/types',                    'POST'),
    ('al_types', '/al/api/v1/admin/types/:typeName/deprecate','POST'),
    ('al_types', '/al/api/v1/admin/types/:typeName/schema',   'POST'),
    ('al_data',  '/al/api/v1/data/:typeName',                 'GET'),
    ('al_data',  '/al/api/v1/data/:typeName/:id',             'GET'),
    ('al_data',  '/al/api/v1/data/:typeName/export',          'GET'),
    ('al_data',  '/al/api/v1/data/:typeName',                 'POST'),
    ('al_data',  '/al/api/v1/data/:typeName/:id/update',      'POST'),
    ('al_data',  '/al/api/v1/data/:typeName/:id/delete',      'POST'),
    ('al_data',  '/al/api/v1/data/:typeName/:id/restore',     'POST'),
    ('al_data',  '/al/api/v1/data/:typeName/import',          'POST')
) AS v(menu_code, api_path, api_method) ON m.code = v.menu_code
ON CONFLICT DO NOTHING;

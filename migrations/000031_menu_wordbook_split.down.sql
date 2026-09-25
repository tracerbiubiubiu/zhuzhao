-- down：整组还原（按钮行/GET 复制/绑定删除 + 挂载回页面 + 三按钮行恢复 + admin 补齐撤）
-- 顺序与 up 严格逆序。

-- ⑥ 撤 admin/superadmin 本次新增绑定（保留 000002 原有）
DELETE FROM role_menus
WHERE role_id IN (SELECT id FROM roles WHERE code IN ('admin','superadmin'))
  AND menu_id IN (
    SELECT id FROM menus WHERE code IN (
      'ticket_type_manage','task_center','al_data','al_types','audit_log',
      'task_submit_btn','task_read_btn','task_manage_btn','task_operate_btn',
      'al_data_write_btn','al_type_manage_btn','ticket_relation_btn','ticket_type_write_btn'));

-- ⑤ 恢复三按钮行与其 menu_apis（POST 三行）——system_menu 页面行未动
INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, v.permission, v.sort, true
FROM (VALUES
    ('system_menu', 'system_menu_create', '登记菜单', 'menu:create', 1),
    ('system_menu', 'system_menu_update', '编辑菜单', 'menu:update', 2),
    ('system_menu', 'system_menu_delete', '删除菜单', 'menu:delete', 3)
) AS v(page_code, code, name, permission, sort)
JOIN menus p ON p.code = v.page_code
ON CONFLICT DO NOTHING;
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, 'POST'
FROM (VALUES
    ('/api/v1/menus'),
    ('/api/v1/menus/update'),
    ('/api/v1/menus/delete')
) AS v(api_path)
JOIN menus m ON m.code = 'system_menu'
ON CONFLICT DO NOTHING;

-- ④ 撤 4 元数据 GET 复制（只删 ticket_type_manage 页面行的四个 GET）
DELETE FROM menu_apis
WHERE menu_id = (SELECT id FROM menus WHERE code = 'ticket_type_manage')
  AND api_method = 'GET'
  AND api_path IN ('/api/v1/ticket-types','/api/v1/ticket-types/:code/fields',
                   '/api/v1/ticket-templates','/api/v1/ticket-templates/:code');

-- ③ 挂载回页面（对照表逆映射：按钮行 → 原页面）+ GET /jobs 回 task_center
UPDATE menu_apis ma
SET menu_id = page.id
FROM menus btn
JOIN LATERAL (VALUES
    ('system_user', '/api/v1/users',                'system_user_create'),
    ('system_user', '/api/v1/users/update',         'system_user_update'),
    ('system_user', '/api/v1/users/delete',         'system_user_delete'),
    ('system_user', '/api/v1/users/status',         'system_user_status'),
    ('system_user', '/api/v1/users/password/reset', 'system_user_reset_pwd'),
    ('system_user', '/api/v1/users/roles',          'system_user_assign_role'),
    ('system_user', '/api/v1/users/orgs',           'system_user_assign_org'),
    ('system_role', '/api/v1/roles',                'system_role_create'),
    ('system_role', '/api/v1/roles/update',         'system_role_update'),
    ('system_role', '/api/v1/roles/delete',         'system_role_delete'),
    ('system_role', '/api/v1/roles/menus',          'system_role_assign_menu'),
    ('system_org',  '/api/v1/orgs',                 'system_org_create'),
    ('system_org',  '/api/v1/orgs/update',          'system_org_update'),
    ('system_org',  '/api/v1/orgs/move',            'system_org_move'),
    ('system_org',  '/api/v1/orgs/delete',          'system_org_delete'),
    ('system_org',  '/api/v1/orgs/members',         'system_org_member'),
    ('system_org',  '/api/v1/orgs/members/delete',  'system_org_member'),
    ('system_org',  '/api/v1/orgs/members/role',    'system_org_member'),
    ('system_org',  '/api/v1/orgs/owners',          'system_org_member'),
    ('task_center', '/api/v1/tasks',                'task_submit_btn'),
    ('task_center', '/api/v1/tasks/cancel',         'task_operate_btn'),
    ('task_center', '/api/v1/tasks/retry',          'task_operate_btn'),
    ('task_center', '/api/v1/jobs',                 'task_manage_btn'),
    ('task_center', '/api/v1/jobs/update',          'task_manage_btn'),
    ('task_center', '/api/v1/jobs/trigger',         'task_manage_btn'),
    ('ticket_list', '/api/v1/tickets',              'ticket_create_btn'),
    ('ticket_list', '/api/v1/tickets/update',       'ticket_update_btn'),
    ('ticket_list', '/api/v1/tickets/close',        'ticket_close_btn'),
    ('ticket_list', '/api/v1/tickets/assign',       'ticket_assign_btn'),
    ('ticket_list', '/api/v1/tickets/delete',       'ticket_delete_btn'),
    ('ticket_list', '/api/v1/tickets/comments',     'ticket_comment_btn'),
    ('ticket_list', '/api/v1/tickets/notes',        'ticket_note_btn'),
    ('ticket_list', '/api/v1/tickets/relations',    'ticket_relation_btn'),
    ('ticket_type_manage', '/api/v1/ticket-types',                 'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-types/update',          'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-types/delete',          'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-types/fields/replace',  'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-templates',             'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-templates/update',      'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-templates/delete',      'ticket_type_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName',                  'al_data_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName/:id/update',       'al_data_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName/:id/delete',       'al_data_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName/:id/restore',      'al_data_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName/import',           'al_data_write_btn'),
    ('al_types', '/al/api/v1/admin/types',                    'al_type_manage_btn'),
    ('al_types', '/al/api/v1/admin/types/:typeName/deprecate','al_type_manage_btn'),
    ('al_types', '/al/api/v1/admin/types/:typeName/schema',   'al_type_manage_btn')
) AS v(page_code, api_path, btn_code) ON true
JOIN menus page ON page.code = v.page_code
WHERE ma.menu_id = btn.id AND ma.api_path = v.api_path;

UPDATE menu_apis ma
SET menu_id = page.id
FROM menus page
WHERE page.code = 'task_center' AND ma.api_path = '/api/v1/jobs' AND ma.api_method = 'GET';

-- ②③ 撤 intent-preserving 新增绑定无法精确区分（ON CONFLICT 防重）——按对照表全量撤按钮绑定
-- （非 admin 角色对三新按钮/写按钮的绑定整体移除；原页面绑定未动）
DELETE FROM role_menus
WHERE role_id NOT IN (SELECT id FROM roles WHERE code IN ('admin','superadmin'))
  AND menu_id IN (SELECT id FROM menus WHERE menu_type = 3
      AND code IN ('ticket_relation_btn','task_operate_btn','ticket_type_write_btn',
                   'system_user_create','system_user_update','system_user_delete','system_user_status',
                   'system_user_reset_pwd','system_user_assign_role','system_user_assign_org',
                   'system_role_create','system_role_update','system_role_delete','system_role_assign_menu',
                   'system_org_create','system_org_update','system_org_move','system_org_delete','system_org_member',
                   'task_submit_btn','task_manage_btn',
                   'ticket_create_btn','ticket_update_btn','ticket_close_btn','ticket_assign_btn',
                   'ticket_delete_btn','ticket_comment_btn','ticket_note_btn',
                   'al_data_write_btn','al_type_manage_btn'));

-- ① 删三新按钮行（其 menu_apis 已随③ 回页面）
DELETE FROM role_menus WHERE menu_id IN (
    SELECT id FROM menus WHERE code IN ('ticket_relation_btn','task_operate_btn','ticket_type_write_btn'));
DELETE FROM menus WHERE code IN ('ticket_relation_btn','task_operate_btn','ticket_type_write_btn');

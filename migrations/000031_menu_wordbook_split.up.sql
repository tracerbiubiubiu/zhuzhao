-- W1（P4-W1 词表重排，B 案）：页面菜单=该页读 API，写 API 挂对应按钮行。
-- 语义：勾页面=只读入口；勾页面+按钮=读写（RuoYi 勾选树/GitHub 每资源两档）。
-- admin/superadmin 策略来自 000002 通配（rbac_service 跳过 menu_apis 路径）零影响；
-- 模型统一后任意自定义角色天然支持只读。
-- ⚠ 读按钮（*_read_btn 等）重排后零 menu_apis 行=设计预期勿补（读权限由页面承载）。
-- ⚠ 同 path 跨 method 红线：本迁移 UPDATE 均按 menu_id 精确定位（非 path 匹配），
--   不触碰 000010 的四个 ticket_list 元数据 GET 行。

-- ============ 路由 → 按钮对照表（B 案 SSOT，验收按此逐行核） ============
-- system_user:  POST /users→user:create | /users/update→user:update | /users/delete→user:delete
--               | /users/status→user:status | /users/password/reset→user:reset_password
--               | /users/roles→user:assign_role | /users/orgs→user:assign_org
-- system_role:  POST /roles→role:create | /roles/update→role:update | /roles/delete→role:delete
--               | /roles/menus→role:assign_menu
-- system_org:   POST /orgs→org:create | /orgs/update→org:update | /orgs/move→org:move
--               | /orgs/delete→org:delete | /orgs/members·members/delete·members/role·owners→org:member（4 行同按钮，豁免行装饰性注记）
-- task_center:  POST /tasks→task:submit | /tasks/cancel·/tasks/retry→task:operate（新）
--               | POST /jobs·/jobs/update·/jobs/trigger·GET /jobs→task:manage（4 行，含 GET——/jobs* 管理面整体）
-- ticket_list:  POST /tickets→ticket:create | /tickets/update→ticket:update | /tickets/close→ticket:close
--               | /tickets/assign→ticket:assign | /tickets/delete→ticket:delete
--               | /tickets/comments→ticket:comment | /tickets/notes→ticket:note
--               | /tickets/relations→ticket:relation（新）
-- ticket_type_manage: 7 写全→ticket:type:manage（新 ticket_type_write_btn，复用页面 permission 字面值）
-- al_data:      5 写全→activelist:data:write
-- al_types:     3 写全→activelist:type:manage

-- ============ ① 三新按钮行 ============
INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, v.permission, v.sort, true
FROM (VALUES
    ('ticket_list',        'ticket_relation_btn',  '关联工单',   'ticket:relation',    10),
    ('task_center',        'task_operate_btn',     '取消/重试',  'task:operate',       4),
    ('ticket_type_manage', 'ticket_type_write_btn','类型配置写', 'ticket:type:manage', 1)
) AS v(page_code, code, name, permission, sort)
JOIN menus p ON p.code = v.page_code
ON CONFLICT DO NOTHING;

-- ============ ② intent-preserving（防重存降权，空转防御） ============
-- 改挂前：凡非 admin 角色经页面绑定 + casbin 含该页写路径 → 按新形态补对应按钮绑定
-- （AssignMenus 替换语义+重算 casbin——不补则重存角色时写权限静默消失）。
INSERT INTO role_menus (role_id, menu_id)
SELECT DISTINCT rm.role_id, btn.id
FROM role_menus rm
JOIN menus page ON page.id = rm.menu_id AND page.menu_type IN (1, 2) AND page.deleted_at IS NULL
JOIN roles r ON r.id = rm.role_id AND r.deleted_at IS NULL AND r.status = 1
            AND r.code NOT IN ('admin', 'superadmin')
JOIN menu_apis ma ON ma.menu_id = page.id AND ma.api_method != 'GET'
JOIN casbin_rule cr ON cr.v1 = ma.api_path AND cr.v2 = ma.api_method
JOIN (VALUES
    ('/api/v1/users',                   'system_user_create'),
    ('/api/v1/users/update',            'system_user_update'),
    ('/api/v1/users/delete',            'system_user_delete'),
    ('/api/v1/users/status',            'system_user_status'),
    ('/api/v1/users/password/reset',    'system_user_reset_pwd'),
    ('/api/v1/users/roles',             'system_user_assign_role'),
    ('/api/v1/users/orgs',              'system_user_assign_org'),
    ('/api/v1/roles',                   'system_role_create'),
    ('/api/v1/roles/update',            'system_role_update'),
    ('/api/v1/roles/delete',            'system_role_delete'),
    ('/api/v1/roles/menus',             'system_role_assign_menu'),
    ('/api/v1/orgs',                    'system_org_create'),
    ('/api/v1/orgs/update',             'system_org_update'),
    ('/api/v1/orgs/move',               'system_org_move'),
    ('/api/v1/orgs/delete',             'system_org_delete'),
    ('/api/v1/orgs/members',            'system_org_member'),
    ('/api/v1/orgs/members/delete',     'system_org_member'),
    ('/api/v1/orgs/members/role',       'system_org_member'),
    ('/api/v1/orgs/owners',             'system_org_member'),
    ('/api/v1/tasks',                   'task_submit_btn'),
    ('/api/v1/tasks/cancel',            'task_operate_btn'),
    ('/api/v1/tasks/retry',             'task_operate_btn'),
    ('/api/v1/jobs',                    'task_manage_btn'),
    ('/api/v1/jobs/update',             'task_manage_btn'),
    ('/api/v1/jobs/trigger',            'task_manage_btn'),
    ('/api/v1/tickets',                 'ticket_create_btn'),
    ('/api/v1/tickets/update',          'ticket_update_btn'),
    ('/api/v1/tickets/close',           'ticket_close_btn'),
    ('/api/v1/tickets/assign',          'ticket_assign_btn'),
    ('/api/v1/tickets/delete',          'ticket_delete_btn'),
    ('/api/v1/tickets/comments',        'ticket_comment_btn'),
    ('/api/v1/tickets/notes',           'ticket_note_btn'),
    ('/api/v1/tickets/relations',       'ticket_relation_btn'),
    ('/api/v1/ticket-types',            'ticket_type_write_btn'),
    ('/api/v1/ticket-types/update',     'ticket_type_write_btn'),
    ('/api/v1/ticket-types/delete',     'ticket_type_write_btn'),
    ('/api/v1/ticket-types/fields/replace', 'ticket_type_write_btn'),
    ('/api/v1/ticket-templates',        'ticket_type_write_btn'),
    ('/api/v1/ticket-templates/update', 'ticket_type_write_btn'),
    ('/api/v1/ticket-templates/delete', 'ticket_type_write_btn'),
    ('/al/api/v1/data/:typeName',               'al_data_write_btn'),
    ('/al/api/v1/data/:typeName/:id/update',    'al_data_write_btn'),
    ('/al/api/v1/data/:typeName/:id/delete',    'al_data_write_btn'),
    ('/al/api/v1/data/:typeName/:id/restore',   'al_data_write_btn'),
    ('/al/api/v1/data/:typeName/import',        'al_data_write_btn'),
    ('/al/api/v1/admin/types',                  'al_type_manage_btn'),
    ('/al/api/v1/admin/types/:typeName/deprecate', 'al_type_manage_btn'),
    ('/al/api/v1/admin/types/:typeName/schema',    'al_type_manage_btn')
) AS m(api_path, btn_code) ON m.api_path = ma.api_path
JOIN menus btn ON btn.code = m.btn_code AND btn.deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- ============ ③ 挂载重排（46 行：页面写行 → 按钮行） ============
UPDATE menu_apis ma
SET menu_id = btn.id
FROM menus page
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
    ('ticket_type_manage', '/api/v1/ticket-types',              'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-types/update',       'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-types/delete',       'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-types/fields/replace', 'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-templates',          'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-templates/update',   'ticket_type_write_btn'),
    ('ticket_type_manage', '/api/v1/ticket-templates/delete',   'ticket_type_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName',                  'al_data_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName/:id/update',       'al_data_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName/:id/delete',       'al_data_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName/:id/restore',      'al_data_write_btn'),
    ('al_data', '/al/api/v1/data/:typeName/import',           'al_data_write_btn'),
    ('al_types', '/al/api/v1/admin/types',                    'al_type_manage_btn'),
    ('al_types', '/al/api/v1/admin/types/:typeName/deprecate','al_type_manage_btn'),
    ('al_types', '/al/api/v1/admin/types/:typeName/schema',   'al_type_manage_btn')
) AS v(page_code, api_path, btn_code) ON true
JOIN menus btn ON btn.code = v.btn_code AND btn.deleted_at IS NULL
WHERE page.code = v.page_code AND page.deleted_at IS NULL
  AND ma.menu_id = page.id AND ma.api_path = v.api_path;

-- GET /jobs 一并归 task_manage_btn（「/jobs* 管理面除外」语义：只勾页面不见 job 定义）
UPDATE menu_apis ma
SET menu_id = btn.id
FROM menus btn
WHERE btn.code = 'task_manage_btn' AND btn.deleted_at IS NULL
  AND ma.api_path = '/api/v1/jobs' AND ma.api_method = 'GET';

-- ============ ④ 4 元数据 GET 双页绑定（ticket_type_manage 页面行复制） ============
-- 只增不删：ticket_list 保留（operator 发起表单 schema 拉取），类型页复制（类型页自洽）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT ttm.id, src.api_path, 'GET'
FROM menus ttm
JOIN menu_apis src ON src.api_path IN (
    '/api/v1/ticket-types', '/api/v1/ticket-types/:code/fields',
    '/api/v1/ticket-templates', '/api/v1/ticket-templates/:code'
) AND src.api_method = 'GET'
JOIN menus srcpage ON srcpage.id = src.menu_id AND srcpage.code = 'ticket_list'
WHERE ttm.code = 'ticket_type_manage'
ON CONFLICT DO NOTHING;

-- ============ ⑤ 菜单写接口孤儿清理（三 menu_apis + 三按钮行 + 绑定） ============
DELETE FROM role_menus WHERE menu_id IN (
    SELECT id FROM menus WHERE code IN ('system_menu_create', 'system_menu_update', 'system_menu_delete'));
-- 只删三行 POST（GET /menus 树保留=角色分配数据源；GET /menus/:id 同留）
DELETE FROM menu_apis WHERE api_method = 'POST' AND menu_id IN (
    SELECT id FROM menus WHERE code = 'system_menu');
DELETE FROM menus WHERE code IN ('system_menu_create', 'system_menu_update', 'system_menu_delete');

-- ============ ⑥ role_menus 缺口修复：admin/superadmin 全量补齐 ============
-- 000018/22/24/25 只插 menus/menu_apis 零绑定——GetUserMenus INNER JOIN 无 admin
-- 旁路（GetUserPermissions 有 B4-4）→「有码无路」；此处对齐 000002「全部菜单」语义。
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id
FROM roles r
CROSS JOIN menus m
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL
  AND m.deleted_at IS NULL
  AND m.code != 'system_menu'  -- 页面行随接口删除保留（GET 树仍需）——仅排孤儿按钮
  AND NOT EXISTS (SELECT 1 FROM role_menus rm WHERE rm.role_id = r.id AND rm.menu_id = m.id)
ON CONFLICT DO NOTHING;

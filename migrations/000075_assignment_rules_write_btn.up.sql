-- 000075: 分派规则写端点迁移按钮行（L1 越权修复）+ 非管理角色 casbin 清除
-- 缺陷（E2E 补齐批二程实证）：000070 把 /assignment-rules 四端点（含三写）绑在 ticket_type_manage
-- **页面行**上，000073 照抄到 ticket_assignment_rules——违反 W1 B 案「页面=读 API、写端点挂按钮」
-- （000018/000010 同域先例：POST /ticket-types 挂 ticket_type_write_btn、POST /tickets 挂
-- ticket_create_btn）。后果：viewer/operator 预设只绑页面即持三写端点 casbin——只读角色可经 API
-- 建/停/删自动分派规则（POST /assignment-rules 实测 400 过 L1 而非 403）。
-- 修复：①三写端点从两页面行摘除，落新按钮行 ticket_assignment_rules_write_btn
-- （menu_type=3，权限码沿用 ticket:type:manage）；②admin/superadmin 绑按钮行（000048 惯例）；
-- ③非 admin/superadmin 角色的三写 casbin 行清除（此前经页面误得，非任何管理员本意；
--   运行中 enforcer 内存策略由下一次 AssignMenus/reloadPolicy 收敛——E2E globalSetup 每次重放即触发）。

INSERT INTO menus (parent_id, menu_type, code, name, path, component, permission, sort_order, icon, visible, is_system)
SELECT m.id, 3, 'ticket_assignment_rules_write_btn', '分派规则写', '', '',
       'ticket:type:manage', 1, '', true, true
FROM menus m WHERE m.code = 'ticket_assignment_rules' AND m.deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('/api/v1/assignment-rules',         'POST'),
    ('/api/v1/assignment-rules/delete',  'POST'),
    ('/api/v1/assignment-rules/enabled', 'POST')
) AS v(api_path, api_method)
WHERE m.code = 'ticket_assignment_rules_write_btn' AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = v.api_method);

-- 页面行摘除三写（GET 保留——页面=读 API）
DELETE FROM menu_apis
WHERE api_method = 'POST'
  AND api_path IN ('/api/v1/assignment-rules', '/api/v1/assignment-rules/delete', '/api/v1/assignment-rules/enabled')
  AND menu_id IN (SELECT id FROM menus WHERE code IN ('ticket_type_manage', 'ticket_assignment_rules'));

-- admin/superadmin 绑按钮行
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r JOIN menus m ON m.code = 'ticket_assignment_rules_write_btn'
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL AND m.deleted_at IS NULL
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- 非管理角色三写 casbin 清除（admin/superadmin 走 matcher 通配不受影响）
DELETE FROM casbin_rule
WHERE ptype = 'p'
  AND (v1, v2) IN (
    ('/api/v1/assignment-rules', 'POST'),
    ('/api/v1/assignment-rules/delete', 'POST'),
    ('/api/v1/assignment-rules/enabled', 'POST'))
  AND v0 NOT IN ('role::admin', 'role::superadmin');

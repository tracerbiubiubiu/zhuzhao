-- Seed data: 4 roles, 3 orgs, admin user, 25 menus, menu_apis, role_menus, Casbin policies

-- ============================================
-- 角色（4 个系统角色）
-- ============================================
INSERT INTO roles (code, name, description, priority, is_system) VALUES
  ('superadmin', '超级管理员', '系统最高权限，可管理管理员', 1, true),
  ('admin', '管理员', '系统管理员，拥有全部权限', 10, true),
  ('operator', '操作员', '可管理组织成员/角色/子组织', 20, true),
  ('viewer', '访客', '只读访问', 30, true)
ON CONFLICT (code) WHERE deleted_at IS NULL DO NOTHING;

-- ============================================
-- 组织（树形，ltree path）
-- ============================================
INSERT INTO organizations (id, code, name, parent_id, path, org_type, is_system, tenant_id) VALUES
  (1, 'root', '集团总部', NULL, 'root', 1, true, 1),
  (2, 'tech', '技术中心', 1, 'root.tech', 2, true, 1),
  (3, 'product', '产品中心', 1, 'root.product', 2, true, 1)
ON CONFLICT (code) WHERE deleted_at IS NULL DO NOTHING;

SELECT setval('organizations_id_seq', (SELECT COALESCE(MAX(id), 0) + 1 FROM organizations));

-- ============================================
-- 超级管理员用户（密码: admin123；工号 E000001）
-- ============================================
INSERT INTO users (username, employee_no, password, real_name, status, is_system, tenant_id)
SELECT 'admin', 'E000001', '$2a$12$YbDKBpLbLGzQEWHzlEFF2.6mhXP5urRNYBIw2WlN25jWSUBwPFXUa', '系统管理员', 1, true, 1
WHERE NOT EXISTS (
  SELECT 1 FROM users WHERE employee_no = 'E000001'
);

-- ============================================
-- 用户-角色绑定（admin 用户绑定 superadmin 角色）
-- ============================================
INSERT INTO user_roles (user_id, role_id)
SELECT u.id, r.id FROM users u, roles r
WHERE u.username = 'admin' AND r.code = 'superadmin'
ON CONFLICT (user_id, role_id) DO NOTHING;

-- ============================================
-- 用户-组织绑定
-- ============================================
INSERT INTO user_orgs (user_id, org_id, is_primary)
SELECT u.id, o.id, true FROM users u, organizations o
WHERE u.username = 'admin' AND o.code = 'root'
ON CONFLICT (user_id, org_id) DO NOTHING;

-- ============================================
-- 初始菜单（目录→菜单→按钮三层）
-- ============================================
INSERT INTO menus (code, name, menu_type, path, component, icon, sort_order, is_system) VALUES
  ('home', '首页', 1, '/home', 'home', 'home', 0, true),
  ('system', '系统管理', 1, '/system', '', 'settings', 1, true),
  ('system_user', '用户管理', 2, '/system/user', 'system/user/index', 'user', 1, true),
  ('system_role', '角色管理', 2, '/system/role', 'system/role/index', 'role', 2, true),
  ('system_menu', '菜单管理', 2, '/system/menu', 'system/menu/index', 'menu', 3, true),
  ('system_org', '组织管理', 2, '/system/org', 'system/org/index', 'org', 4, true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, v.permission, v.sort_order, true
FROM (VALUES
  ('system_user', 'system_user_create', '新建用户', 'user:create', 1),
  ('system_user', 'system_user_update', '编辑用户', 'user:update', 2),
  ('system_user', 'system_user_delete', '删除用户', 'user:delete', 3),
  ('system_user', 'system_user_status', '启用/禁用', 'user:status', 4),
  ('system_user', 'system_user_reset_pwd', '重置密码', 'user:reset_password', 5),
  ('system_user', 'system_user_assign_role', '分配角色', 'user:assign_role', 6),
  ('system_user', 'system_user_assign_org', '分配组织', 'user:assign_org', 7),
  ('system_role', 'system_role_create', '新建角色', 'role:create', 1),
  ('system_role', 'system_role_update', '编辑角色', 'role:update', 2),
  ('system_role', 'system_role_delete', '删除角色', 'role:delete', 3),
  ('system_role', 'system_role_assign_menu', '分配菜单', 'role:assign_menu', 4),
  ('system_menu', 'system_menu_create', '登记菜单', 'menu:create', 1),
  ('system_menu', 'system_menu_update', '编辑菜单', 'menu:update', 2),
  ('system_menu', 'system_menu_delete', '删除菜单', 'menu:delete', 3),
  ('system_org', 'system_org_create', '新建组织', 'org:create', 1),
  ('system_org', 'system_org_update', '编辑组织', 'org:update', 2),
  ('system_org', 'system_org_delete', '删除组织', 'org:delete', 3),
  ('system_org', 'system_org_move', '移动组织', 'org:move', 4),
  ('system_org', 'system_org_member', '成员管理', 'org:member', 5)
) AS v(parent_code, code, name, permission, sort_order)
JOIN menus p ON p.code = v.parent_code
ON CONFLICT (code) DO NOTHING;

-- ============================================
-- 菜单-API 绑定（用于 Casbin 策略生成）
-- ============================================
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m, (VALUES
  ('system_user', '/api/v1/users', 'GET'),
  ('system_user', '/api/v1/users', 'POST'),
  ('system_user', '/api/v1/users/:id', 'GET'),
  ('system_user', '/api/v1/users/update', 'POST'),
  ('system_user', '/api/v1/users/delete', 'POST'),
  ('system_user', '/api/v1/users/status', 'POST'),
  ('system_user', '/api/v1/users/roles', 'POST'),
  ('system_user', '/api/v1/users/orgs', 'POST'),
  ('system_user', '/api/v1/users/:id/orgs', 'GET'),
  ('system_user', '/api/v1/users/password/reset', 'POST'),
  ('system_role', '/api/v1/roles', 'GET'),
  ('system_role', '/api/v1/roles', 'POST'),
  ('system_role', '/api/v1/roles/:id', 'GET'),
  ('system_role', '/api/v1/roles/update', 'POST'),
  ('system_role', '/api/v1/roles/delete', 'POST'),
  ('system_role', '/api/v1/roles/:id/menus', 'GET'),
  ('system_role', '/api/v1/roles/menus', 'POST'),
  ('system_role', '/api/v1/roles/:id/permissions', 'GET'),
  ('system_menu', '/api/v1/menus', 'GET'),
  ('system_menu', '/api/v1/menus', 'POST'),
  ('system_menu', '/api/v1/menus/:id', 'GET'),
  ('system_menu', '/api/v1/menus/update', 'POST'),
  ('system_menu', '/api/v1/menus/delete', 'POST'),
  ('system_org', '/api/v1/orgs', 'GET'),
  ('system_org', '/api/v1/orgs', 'POST'),
  ('system_org', '/api/v1/orgs/:id', 'GET'),
  ('system_org', '/api/v1/orgs/update', 'POST'),
  ('system_org', '/api/v1/orgs/delete', 'POST'),
  ('system_org', '/api/v1/orgs/move', 'POST'),
  ('system_org', '/api/v1/orgs/:id/members', 'GET'),
  ('system_org', '/api/v1/orgs/members', 'POST'),
  ('system_org', '/api/v1/orgs/members/delete', 'POST')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code
ON CONFLICT DO NOTHING;

-- ============================================
-- 角色-菜单绑定（superadmin + admin：全部 IAM 菜单）
-- ============================================
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r, menus m
WHERE r.code IN ('superadmin', 'admin')
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- ============================================
-- Casbin 路由级策略：admin + superadmin 通配
-- ============================================
INSERT INTO casbin_rule (ptype, v0, v1, v2) VALUES
  ('p', 'role::admin', '*', '*'),
  ('p', 'role::superadmin', '*', '*')
ON CONFLICT DO NOTHING;

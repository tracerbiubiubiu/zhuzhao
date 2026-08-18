DELETE FROM role_menus WHERE role_id IN (
  SELECT id FROM roles WHERE is_system = true
);
DELETE FROM menu_apis WHERE menu_id IN (
  SELECT id FROM menus WHERE is_system = true
);
DELETE FROM user_roles WHERE role_id IN (
  SELECT id FROM roles WHERE is_system = true
);
DELETE FROM user_orgs WHERE user_id IN (
  SELECT id FROM users WHERE is_system = true
);
DELETE FROM casbin_rule WHERE v0 IN ('role::admin', 'role::superadmin', 'role::operator', 'role::viewer');
DELETE FROM menus WHERE is_system = true;
DELETE FROM users WHERE is_system = true;
DELETE FROM organizations WHERE is_system = true;
DELETE FROM roles WHERE is_system = true;

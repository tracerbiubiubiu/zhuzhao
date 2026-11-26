INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT id, 'risk_remediation_accept_rework_btn', '接续到期风险接受', 3, 'risk:manage', 13, true
FROM menus WHERE code = 'risk_list';

INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r, menus m
WHERE r.deleted_at IS NULL AND r.code IN ('admin', 'superadmin')
  AND m.code = 'risk_remediation_accept_rework_btn';

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT id, '/api/v1/risk-items/remediation-accept-rework', 'POST'
FROM menus WHERE code = 'risk_remediation_accept_rework_btn';

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, a.api_path, a.api_method
FROM roles r JOIN role_menus rm ON rm.role_id = r.id
JOIN menu_apis a ON a.menu_id = rm.menu_id
WHERE r.deleted_at IS NULL AND a.api_path = '/api/v1/risk-items/remediation-accept-rework';

DELETE FROM casbin_rule WHERE ptype = 'p' AND v1 = '/api/v1/risk-items/remediation-accept-rework' AND v2 = 'POST';
DELETE FROM menu_apis WHERE api_path = '/api/v1/risk-items/remediation-accept-rework' AND api_method = 'POST';
DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE code = 'risk_remediation_accept_rework_btn');
DELETE FROM menus WHERE code = 'risk_remediation_accept_rework_btn';

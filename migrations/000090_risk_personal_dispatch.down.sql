-- Preserve issued tickets, associations and the original operation binding.
LOCK TABLE risk_dispatch_group_receipts, risk_ticket_links, risk_dispatch_groups,
    risk_dispatch_operations, risk_dispatch_plans, risk_dispatch_targets,
    risk_operations IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM risk_dispatch_group_receipts)
        OR EXISTS (SELECT 1 FROM risk_ticket_links)
        OR EXISTS (SELECT 1 FROM risk_dispatch_groups)
        OR EXISTS (SELECT 1 FROM risk_dispatch_operations)
        OR EXISTS (SELECT 1 FROM risk_operations WHERE command = 'dispatch') THEN
        RAISE EXCEPTION 'risk dispatch facts exist; retain history and use a compatible forward migration';
    END IF;
END;
$$;

DELETE FROM casbin_rule WHERE ptype = 'p' AND v2 = 'GET'
    AND v1 IN ('/api/v1/risk-items/dispatch-operations/:id', '/api/v1/risk-items/dispatch-statistics');
DELETE FROM menu_apis WHERE api_method = 'GET' AND api_path IN ('/api/v1/risk-items/dispatch-operations/:id', '/api/v1/risk-items/dispatch-statistics')
    AND menu_id IN (SELECT id FROM menus WHERE code = 'risk_list');
DELETE FROM casbin_rule WHERE ptype = 'p' AND v2 = 'POST' AND v1 IN (
    '/api/v1/risk-items/dispatch-plan', '/api/v1/risk-items/dispatch', '/api/v1/risk-items/batch-dispatch'
);
DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE code = 'risk_dispatch_btn');
DELETE FROM menu_apis WHERE menu_id IN (SELECT id FROM menus WHERE code = 'risk_dispatch_btn');
DELETE FROM menus WHERE code = 'risk_dispatch_btn';

DROP TABLE risk_dispatch_group_receipts;
DROP TABLE risk_ticket_links;
DROP TABLE risk_dispatch_groups;
DROP TABLE risk_dispatch_operations;
DROP TABLE risk_dispatch_plans;
DROP TABLE risk_dispatch_targets;

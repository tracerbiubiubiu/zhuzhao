-- Keep durable risk observations and operations intact with their domain facts.
LOCK TABLE risk_events, risk_operations, risk_ingest_conflicts, risk_observations,
    risk_source_records, risk_items, risk_families, risk_sources IN ACCESS EXCLUSIVE MODE;

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM risk_events)
        OR EXISTS (SELECT 1 FROM risk_operations)
        OR EXISTS (SELECT 1 FROM risk_ingest_conflicts)
        OR EXISTS (SELECT 1 FROM risk_observations)
        OR EXISTS (SELECT 1 FROM risk_source_records)
        OR EXISTS (SELECT 1 FROM risk_items)
        OR EXISTS (SELECT 1 FROM risk_families) THEN
        RAISE EXCEPTION 'risk registration facts exist; retain history and use a compatible forward migration';
    END IF;
END;
$$;

DELETE FROM casbin_rule
WHERE ptype = 'p' AND (v1, v2) IN (
    ('/api/v1/risk-items',                     'GET'),
    ('/api/v1/risk-items/:id',                 'GET'),
    ('/api/v1/risk-items/:id/history',         'GET'),
    ('/api/v1/risk-items/:id/asset',           'GET'),
    ('/api/v1/risk-items/ingest-receipts/:id', 'GET'),
    ('/api/v1/risk-items/operations/:id',      'GET'),
    ('/api/v1/risk-items/create',              'POST'),
    ('/api/v1/risk-items/assess',              'POST'),
    ('/api/v1/risk-items/assign',              'POST'));

DELETE FROM role_menus
WHERE menu_id IN (SELECT id FROM menus WHERE code IN (
    'risk_manage', 'risk_list',
    'risk_create_btn', 'risk_assess_btn', 'risk_assign_btn'));

DELETE FROM menu_apis
WHERE menu_id IN (SELECT id FROM menus WHERE code IN (
    'risk_manage', 'risk_list',
    'risk_create_btn', 'risk_assess_btn', 'risk_assign_btn'));

DELETE FROM menus WHERE code IN (
    'risk_create_btn', 'risk_assess_btn', 'risk_assign_btn',
    'risk_list', 'risk_manage');

DROP TABLE IF EXISTS risk_events;
DROP TABLE IF EXISTS risk_operations;
DROP TABLE IF EXISTS risk_ingest_conflicts;
DROP TABLE IF EXISTS risk_observations;
DROP TABLE IF EXISTS risk_source_records;
DROP TABLE IF EXISTS risk_items;
DROP TABLE IF EXISTS risk_families;
DROP TABLE IF EXISTS risk_sources;

-- 还原
ALTER TABLE audit_logs DROP COLUMN IF EXISTS response_summary;

DROP TABLE IF EXISTS panic_logs;

DELETE FROM menu_apis WHERE (api_path, api_method) IN ((
    '/api/v1/audit/panics', 'GET'), (
    '/api/v1/audit/reconcile', 'GET'));

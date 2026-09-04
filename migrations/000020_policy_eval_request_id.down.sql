DROP INDEX IF EXISTS idx_ticket_events_request;
ALTER TABLE ticket_events DROP COLUMN IF EXISTS request_id;

DROP INDEX IF EXISTS idx_audit_request;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS request_id;

DROP INDEX IF EXISTS idx_pel_trace;
DROP INDEX IF EXISTS idx_pel_actor_created;
DROP INDEX IF EXISTS idx_pel_created;
DROP TABLE IF EXISTS policy_evaluation_logs;

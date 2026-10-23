DROP INDEX IF EXISTS idx_tickets_human_key;
ALTER TABLE tickets DROP COLUMN IF EXISTS human_key;

-- 000027 down: 回滚三表乐观锁列（P1-3）
ALTER TABLE ticket_types        DROP COLUMN IF EXISTS version;
ALTER TABLE ticket_type_fields  DROP COLUMN IF EXISTS version;
ALTER TABLE ticket_templates    DROP COLUMN IF EXISTS version;

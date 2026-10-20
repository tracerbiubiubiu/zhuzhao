-- 000047 down: 删两可空列（零回填列，无数据迁移语义；存量 NULL 直接丢弃）
ALTER TABLE file_objects
    DROP COLUMN IF EXISTS owner_id,
    DROP COLUMN IF EXISTS owner_kind;

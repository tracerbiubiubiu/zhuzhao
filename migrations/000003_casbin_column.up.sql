-- 对齐 pckhoi/casbin-pgx-adapter/v3 列名（p_type）
DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'casbin_rule'
      AND column_name = 'ptype'
  ) THEN
    ALTER TABLE casbin_rule RENAME COLUMN ptype TO p_type;
  END IF;
END $$;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM information_schema.columns
    WHERE table_schema = current_schema()
      AND table_name = 'casbin_rule'
      AND column_name = 'p_type'
  ) THEN
    ALTER TABLE casbin_rule RENAME COLUMN p_type TO ptype;
  END IF;
END $$;

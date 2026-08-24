-- 对齐 noho-digital/casbin-pgx-adapter 列名（ptype）
-- migration 000003 曾将 ptype 重命名为 p_type（对齐 pckhoi adapter）
-- 迁移到 noho-digital adapter 后需要改回 ptype
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

-- 重建唯一索引（noho-digital adapter 使用 ptype 列名）
DROP INDEX IF EXISTS idx_casbin_rule;
CREATE UNIQUE INDEX IF NOT EXISTS idx_casbin_rule ON casbin_rule (ptype, COALESCE(v0,''), COALESCE(v1,''), COALESCE(v2,''), COALESCE(v3,''), COALESCE(v4,''), COALESCE(v5,''));

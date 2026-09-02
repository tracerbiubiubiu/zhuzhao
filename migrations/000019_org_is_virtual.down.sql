-- 000019 down：重建 org_type（有损——1/2/3 历史细分已不可恢复，统一回落 3=部门）。
-- 幂等写法（与 up 同理，runMigration 全量重执行）。
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS org_type SMALLINT NOT NULL DEFAULT 3;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'organizations' AND column_name = 'is_virtual') THEN
        UPDATE organizations SET org_type = CASE WHEN is_virtual THEN 4 ELSE 3 END;
        ALTER TABLE organizations DROP COLUMN is_virtual;
    END IF;
END $$;

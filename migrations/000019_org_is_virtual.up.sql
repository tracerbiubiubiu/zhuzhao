-- 000019: org_type 四值枚举（1=公司 2=部门 3=小组 4=虚拟组）收敛为布尔 is_virtual。
-- 依据（2026-09-01）：全仓行为消费点仅区分「实体 vs 虚拟」二值——
--   scope_resolver 透明读锚点 / BK-13 ticket_visibility 配置 / vg 挂载与删除分流；
--   1/2/3 的层级细分零代码消费（层级由 path/nlevel 表达，展示关注点留展示层）。
-- 字段名与类型一并到位：布尔使「枚举再长出无消费细分」无法复发。
-- 写法幂等（testutil.runMigration 每次全量重执行，与 000018 IF NOT EXISTS 风格一致）。
ALTER TABLE organizations ADD COLUMN IF NOT EXISTS is_virtual BOOLEAN NOT NULL DEFAULT false;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM information_schema.columns
               WHERE table_name = 'organizations' AND column_name = 'org_type') THEN
        UPDATE organizations SET is_virtual = (org_type = 4);
        ALTER TABLE organizations DROP COLUMN org_type;
    END IF;
END $$;

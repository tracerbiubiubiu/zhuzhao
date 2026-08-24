-- 将 casbin_rule 表 v2-v5 列从 DEFAULT '' 改为 NULL
-- noho-digital adapter 用 sql.NullString 扫描，空字符串会被当作有效值
-- 导致 persist.LoadPolicyLine 收到多余的空参数，影响策略匹配
ALTER TABLE casbin_rule ALTER COLUMN v2 DROP NOT NULL;
ALTER TABLE casbin_rule ALTER COLUMN v2 DROP DEFAULT;
ALTER TABLE casbin_rule ALTER COLUMN v3 DROP NOT NULL;
ALTER TABLE casbin_rule ALTER COLUMN v3 DROP DEFAULT;
ALTER TABLE casbin_rule ALTER COLUMN v4 DROP NOT NULL;
ALTER TABLE casbin_rule ALTER COLUMN v4 DROP DEFAULT;
ALTER TABLE casbin_rule ALTER COLUMN v5 DROP NOT NULL;
ALTER TABLE casbin_rule ALTER COLUMN v5 DROP DEFAULT;

-- 将现有空字符串转为 NULL
UPDATE casbin_rule SET v2 = NULL WHERE v2 = '';
UPDATE casbin_rule SET v3 = NULL WHERE v3 = '';
UPDATE casbin_rule SET v4 = NULL WHERE v4 = '';
UPDATE casbin_rule SET v5 = NULL WHERE v5 = '';

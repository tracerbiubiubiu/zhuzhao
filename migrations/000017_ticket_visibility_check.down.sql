-- 000017 down: 移除 project_isolated CHECK（恢复 000011 的无约束态）。
-- 已写入的 project_isolated 值保留（CHECK 移除不影响存量数据），
-- 上层语义回退由应用侧（resolver 锚点门控自动失效）保证。

ALTER TABLE organizations
    DROP CONSTRAINT IF EXISTS organizations_ticket_visibility_check;

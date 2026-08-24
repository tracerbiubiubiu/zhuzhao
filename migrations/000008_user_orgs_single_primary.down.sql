-- B3-3：回滚 primary 互斥索引（存量降级数据不自动恢复）
DROP INDEX IF EXISTS idx_user_orgs_single_primary;

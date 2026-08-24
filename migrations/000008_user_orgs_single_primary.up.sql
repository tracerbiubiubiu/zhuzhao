-- B3-3（R2-ORG-03）：primary 互斥数据库约束兜底。
-- 修复前「同一用户最多一条 is_primary=true」仅靠应用层维护，
-- 并发 AddMember（同 user、不同 org、均 primary）可产生双 primary。

-- 1. 存量修复：每 user 多条 primary 时保留一条（ctid 最小），其余降级
UPDATE user_orgs uo
SET is_primary = false
WHERE uo.is_primary
  AND EXISTS (
    SELECT 1 FROM user_orgs other
    WHERE other.user_id = uo.user_id
      AND other.is_primary
      AND other.ctid < uo.ctid
  );

-- 2. 部分唯一索引：同一用户至多一条 primary（并发第二个事务收 23505 而非脏数据）
CREATE UNIQUE INDEX idx_user_orgs_single_primary
    ON user_orgs (user_id)
    WHERE is_primary;

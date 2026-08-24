-- F-6 修复：唯一索引补软删过滤，软删数据不再永久占用唯一键。
-- 此前 idx_users_employee_no / idx_users_domain_account 缺 WHERE deleted_at IS NULL
--（对比 roles/organizations 的部分唯一索引），软删用户的工号永久无法复用；
-- menus.code 为列级 UNIQUE，软删菜单后同 code 无法重建。
-- 索引名保持不变（pgerr.go 依赖约束名做错误映射）。

DROP INDEX IF EXISTS idx_users_employee_no;
CREATE UNIQUE INDEX idx_users_employee_no ON users(employee_no)
    WHERE employee_no IS NOT NULL AND employee_no <> '' AND deleted_at IS NULL;

DROP INDEX IF EXISTS idx_users_domain_account;
CREATE UNIQUE INDEX idx_users_domain_account ON users(user_domain, domain_account)
    WHERE domain_account IS NOT NULL AND domain_account <> ''
      AND user_domain IS NOT NULL AND user_domain <> ''
      AND deleted_at IS NULL;

-- menus.code 由列级约束改为部分唯一索引（约束名 menus_code_key → idx_menus_code，
-- repository/pgerr.go 的映射已同步更新）
ALTER TABLE menus DROP CONSTRAINT IF EXISTS menus_code_key;
CREATE UNIQUE INDEX idx_menus_code ON menus(code) WHERE deleted_at IS NULL;

-- 清理历史软删行造成的唯一键占用（如有）：已被软删的重复数据让新数据可用
UPDATE users SET employee_no = employee_no || '#del#' || id
    WHERE deleted_at IS NOT NULL AND employee_no IS NOT NULL AND employee_no <> '';
UPDATE users SET domain_account = domain_account || '#del#' || id
    WHERE deleted_at IS NOT NULL AND domain_account IS NOT NULL AND domain_account <> '';

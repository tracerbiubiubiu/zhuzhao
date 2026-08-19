-- 回滚：恢复为不含软删过滤的唯一索引。
-- 尽力而为：up 之后软删行的键可能与活跃行（或彼此）重复——这是 up 后的合法数据状态，
-- 回滚时冲突的软删行加 #del# 后缀让位（活跃行优先；软删行之间最老者保留原值），
-- 否则重建非部分唯一索引会因重复键硬失败，把库置为 dirty。

-- 菜单：冲突的软删行 code 加后缀
UPDATE menus m SET code = m.code || '#del#' || m.id
WHERE m.deleted_at IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM menus o
    WHERE o.code = m.code AND o.id <> m.id
      AND (o.deleted_at IS NULL OR o.id < m.id)
  );
DROP INDEX IF EXISTS idx_menus_code;
ALTER TABLE menus ADD CONSTRAINT menus_code_key UNIQUE (code);

-- 用户工号：先还原被 up 改写的软删行，再对仍冲突的软删行重新加后缀
UPDATE users SET employee_no = split_part(employee_no, '#del#', 1)
    WHERE deleted_at IS NOT NULL AND employee_no LIKE '%#del#%';
UPDATE users u SET employee_no = u.employee_no || '#del#' || u.id
WHERE u.deleted_at IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM users o
    WHERE o.employee_no = u.employee_no AND o.id <> u.id
      AND (o.deleted_at IS NULL OR o.id < u.id)
  );

-- 域账号：同上，冲突判定按 (user_domain, domain_account) 组合
UPDATE users SET domain_account = split_part(domain_account, '#del#', 1)
    WHERE deleted_at IS NOT NULL AND domain_account LIKE '%#del#%';
UPDATE users u SET domain_account = u.domain_account || '#del#' || u.id
WHERE u.deleted_at IS NOT NULL
  AND EXISTS (
    SELECT 1 FROM users o
    WHERE o.domain_account = u.domain_account
      AND o.user_domain IS NOT DISTINCT FROM u.user_domain
      AND o.id <> u.id
      AND (o.deleted_at IS NULL OR o.id < u.id)
  );

DROP INDEX IF EXISTS idx_users_domain_account;
CREATE UNIQUE INDEX idx_users_domain_account ON users(user_domain, domain_account)
    WHERE domain_account IS NOT NULL AND domain_account <> ''
      AND user_domain IS NOT NULL AND user_domain <> '';

DROP INDEX IF EXISTS idx_users_employee_no;
CREATE UNIQUE INDEX idx_users_employee_no ON users(employee_no)
    WHERE employee_no IS NOT NULL AND employee_no <> '';

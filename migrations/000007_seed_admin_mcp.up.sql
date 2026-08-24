-- F-10 存量修复：已初始化环境中的种子 admin 不会触发 000002 的幂等 INSERT，
-- 在此显式开启强制改密（弱初始凭证 admin123 不得长期有效）
UPDATE users SET must_change_password = true, updated_at = NOW()
WHERE username = 'admin' AND is_system = true AND deleted_at IS NULL;

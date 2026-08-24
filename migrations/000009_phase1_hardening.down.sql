-- 索引回滚（描述修正属无意义回滚的数据修复，不还原——见 000009 up 注释）
DROP INDEX IF EXISTS idx_audit_logs_created;
DROP INDEX IF EXISTS idx_roles_status_deleted;
DROP INDEX IF EXISTS idx_user_roles_role;
DROP INDEX IF EXISTS idx_role_menus_menu;
DROP INDEX IF EXISTS idx_user_orgs_org;

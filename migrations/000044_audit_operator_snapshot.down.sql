-- 000044 回滚：删操作人快照两列（精确逆）
ALTER TABLE audit_logs DROP COLUMN IF EXISTS employee_no;
ALTER TABLE audit_logs DROP COLUMN IF EXISTS real_name;

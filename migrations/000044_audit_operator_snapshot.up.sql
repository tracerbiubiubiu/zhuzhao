-- 000044: audit_logs 操作人快照列（employee_no/real_name）
--
-- 行内自描述（2026-10-13 用户展示统一批）：审计行是 append-only 快照，归档导出
-- JSONL 后无 users 表可 join——此前操作人仅存 username（JWT claims 的本地账号名，
-- 既非登录键工号、也非人名），归因可读性缺口。写入时经 users 快照两列（写入时刻
-- 定格：employee_no 恒定；real_name 为动作时刻值，符合审计归因语义）。
-- 增量列方案：不动 username 列，存量行两列为 NULL，读端 COALESCE 兜底。
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS employee_no VARCHAR(50);
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS real_name VARCHAR(100);

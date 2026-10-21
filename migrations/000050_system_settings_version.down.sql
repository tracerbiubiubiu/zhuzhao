-- 000050 down: 删 version 列（乐观锁元数据，无业务数据语义）
ALTER TABLE system_settings DROP COLUMN IF EXISTS version;

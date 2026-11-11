-- 000050: system_settings version 乐观锁（并行审计 F-D2 挂起项收口 2026-10-21）
--
-- 语义：KV 行带版本号，Update 走「读版本→条件更新 version=version+1」CAS——
-- expected_version 非空时严格校验（两个管理员并发改同键，后写者不再静默覆盖
-- 前写者）；存量行回填 version=1（DEFAULT 即回填，无数据迁移语义）。
ALTER TABLE system_settings
    ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

-- 000060: message_rules 追加 version 乐观锁列（微批 A——审计残余③；06 号 A.5
-- 设计原文「version 乐观锁——对齐 notification_configs CAS 全仓惯例」的落地欠账）。
--
-- 语义对齐 000050 system_settings / message_templates 既有 CAS：并发编辑同一
-- (biz_type, event_action) 规则时，后写者携陈旧 version → upsert 的 DO UPDATE
-- WHERE 不命中 → 0 行 → 92506（409）提示刷新，不静默覆盖。存量行回填 1。
ALTER TABLE message_rules ADD COLUMN IF NOT EXISTS version BIGINT NOT NULL DEFAULT 1;

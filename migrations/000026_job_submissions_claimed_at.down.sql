-- 000026 回滚：移除回调抢占时刻列。
ALTER TABLE job_submissions DROP COLUMN IF EXISTS claimed_at;

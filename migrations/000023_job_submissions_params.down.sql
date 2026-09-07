-- 000023 down：params 为排障快照列，删除即可（权威入参在 taskrunner 侧 job 定义与审计）。
ALTER TABLE job_submissions DROP COLUMN IF EXISTS params;

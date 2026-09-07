-- 000023: job_submissions 补 params 快照列（执行可观测——任务提交入参定格，
-- "查单任务详情"的输入侧；zhuzhao 权威全量，taskrunner job_runs.dept/params
-- 为排障快照，2026-09-07 拍板）。
ALTER TABLE job_submissions ADD COLUMN IF NOT EXISTS params TEXT NOT NULL DEFAULT '{}';

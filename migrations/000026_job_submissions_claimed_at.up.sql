-- 000026: 回调幂等栅栏原子化——job_submissions 补 claimed_at「抢占时刻」（P1-1）。
--
-- 背景：原 EnsureCallbackRow = `INSERT ... ON CONFLICT (task_id) DO NOTHING` +
-- 独立 `SELECT` 两段式，两者不在同一原子操作内。并发同 task_id 回调会全部
-- 命中 DO NOTHING 后读到 status='submitted'，导致 service 层 handler 被重复
-- 执行（真机实测 8 并发 → 副作用执行 8 次）。
--
-- 修复：改为单语句原子抢占
--   INSERT ... ON CONFLICT (task_id) DO UPDATE SET status='running', claimed_at=NOW()
--   WHERE status IN ('submitted','failed') OR (running 且 claimed_at 陈旧)
--   并有行返回即取得执行权（唯一），否则 pgx.ErrNoRows（他人已 succeeded /
--   他人正在途 running）。
--
-- claimed_at 用途：记录本次抢占时刻。进程崩溃留下的陈旧 running（claimed_at 超
-- 10 分钟）允许被重认领，实现自愈；正常在途 running 则拦截重复执行。
--
-- 注意：status 取值扩展为 submitted / running / succeeded / failed。该列为
-- VARCHAR(20) NOT NULL DEFAULT 'submitted'，**无 CHECK 约束**，新增 'running'
-- 取值无需额外 DDL。
ALTER TABLE job_submissions ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;

COMMENT ON COLUMN job_submissions.claimed_at IS '回调“抢占”时刻：原子认领时写入 NOW()；用于陈旧 running 的可重认领判定（超 10 分钟视为陈旧）';

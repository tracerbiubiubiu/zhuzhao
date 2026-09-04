-- E-② 内网回调基础设施（16 号 §3）：任务提交日志 + 回调幂等，一表两用。
-- zhuzhao 提交任务（E-④）→ INSERT origin='api'（提交凭证，薄）；
-- taskrunner 回调到达 → task_id 幂等键：已 succeeded 直接 2xx（at-least-once 下防重复执行副作用），
-- 无行则补录 origin='callback'（cron 触发，zhuzhao 未提交过）再执行；失败留 failed 允许重试。

CREATE TABLE job_submissions (
    id           BIGSERIAL PRIMARY KEY,
    task_id      VARCHAR(64) NOT NULL UNIQUE,           -- taskrunner task_id（幂等键，调用方生成）
    request_id   VARCHAR(64),                            -- zhuzhao 侧关联键（cron 触发为空）
    action       VARCHAR(50) NOT NULL,                   -- action_id
    origin       VARCHAR(10) NOT NULL DEFAULT 'callback', -- api（zhuzhao 提交）/ callback（到达时补录）
    status       VARCHAR(20) NOT NULL DEFAULT 'submitted', -- submitted / succeeded / failed
    error        TEXT,
    submitted_by VARCHAR(50),                            -- actor 工号（审计归因，cron 为空）
    source_ip    VARCHAR(50),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    executed_at  TIMESTAMPTZ
);

CREATE INDEX idx_job_submissions_request ON job_submissions(request_id) WHERE request_id IS NOT NULL AND request_id <> '';
CREATE INDEX idx_job_submissions_action_created ON job_submissions(action, created_at DESC);

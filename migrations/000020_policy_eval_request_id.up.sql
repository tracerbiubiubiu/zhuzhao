-- B11① 判定日志表（03-audit-l2 §3.2 DDL）+ 全链路 request_id 关联（03 §3.4）：
-- audit_logs / ticket_events 补 request_id 列，打通 slog=审计=事件=判定日志同一 req- 键。
-- ⚠️ 天然大表：保留期 180 天（可配），归档（B11② audit_archive，随 M-E）先于放量。

CREATE TABLE policy_evaluation_logs (
    id               BIGSERIAL PRIMARY KEY,
    actor_id         BIGINT NOT NULL,             -- 操作人
    actor_role_codes TEXT[],                       -- 角色码（L1 展开结果）
    resource_type    VARCHAR(50) NOT NULL,         -- ticket / builtin:task / ...
    resource_id      VARCHAR(100) NOT NULL,        -- 资源标识（如 ticket:123；端点判定可空串）
    action           VARCHAR(50) NOT NULL,         -- read / update / submit / ...
    scope_axis       VARCHAR(20),                  -- L1 | L2 | L3（判定层；v1 由调用方/埋点填充，可空）
    scope_detail     JSONB,                        -- 解析轴细节（锚点/scope 快照，可空）
    result           BOOLEAN NOT NULL,             -- 允许 / 拒绝
    reason           VARCHAR(200),                 -- 拒绝原因（scope mismatch / 非属主 / error:...）
    trace_id         VARCHAR(64),                  -- = request_id（request context 注入后取）
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_pel_created ON policy_evaluation_logs(created_at);
CREATE INDEX idx_pel_actor_created ON policy_evaluation_logs(actor_id, created_at DESC);
CREATE INDEX idx_pel_trace ON policy_evaluation_logs(trace_id) WHERE trace_id IS NOT NULL AND trace_id <> '';

ALTER TABLE audit_logs ADD COLUMN request_id VARCHAR(64);
CREATE INDEX idx_audit_request ON audit_logs(request_id) WHERE request_id IS NOT NULL AND request_id <> '';

ALTER TABLE ticket_events ADD COLUMN request_id VARCHAR(64);
CREATE INDEX idx_ticket_events_request ON ticket_events(request_id) WHERE request_id IS NOT NULL AND request_id <> '';

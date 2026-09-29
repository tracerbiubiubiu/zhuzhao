-- P4-8 P2 小件打包：panic 落库+聚合查询 / 审计响应体摘要。
-- （只读对账端点与 /metrics 指标为纯代码件无表——不涉本迁移）
CREATE TABLE IF NOT EXISTS panic_logs (
    id           BIGSERIAL PRIMARY KEY,
    fingerprint  CHAR(64)    NOT NULL UNIQUE, -- sha256(message+栈首 3 行)——聚合键
    message      TEXT        NOT NULL,
    stack        TEXT        NOT NULL,
    path         VARCHAR(200) NOT NULL DEFAULT '',
    count        BIGINT      NOT NULL DEFAULT 1, -- 同指纹聚合次数
    first_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_panic_last ON panic_logs (last_at DESC);

-- 审计响应体摘要（截断存储——排查「请求成功但响应异常」类问题；历史行 NULL）
ALTER TABLE audit_logs ADD COLUMN IF NOT EXISTS response_summary TEXT;

-- P4-8 两查询端点绑 audit_log 页面行（audit:read 面——admin 专属既有口径）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m JOIN (VALUES
    ('audit_log', '/api/v1/audit/panics',    'GET'),
    ('audit_log', '/api/v1/audit/reconcile', 'GET')
) AS v(menu_code, api_path, api_method) ON m.code = v.menu_code
ON CONFLICT DO NOTHING;

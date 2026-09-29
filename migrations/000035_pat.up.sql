-- P4-6 个人 API token（PAT——用户侧非交互凭据：脚本/CI 调用）。
-- GitHub PAT 蓝本（design-decisions §26.5）：独立 secret 明文仅创建时返回一次、
-- 落库只存 sha256；可吊销（revoked_at）；scope 字段留位（默认 full——细分
-- 触发驱动，首版执行面不拦）。
CREATE TABLE IF NOT EXISTS personal_access_tokens (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT       NOT NULL REFERENCES users (id),
    name         VARCHAR(128) NOT NULL,
    secret_hash  CHAR(64)     NOT NULL UNIQUE, -- sha256 hex（明文 zpat_* 不落库）
    scope        VARCHAR(32)  NOT NULL DEFAULT 'full', -- 留位：full（细分触发驱动）
    expires_at   TIMESTAMPTZ,                    -- NULL=永不过期（GitHub 同款语义）
    revoked_at   TIMESTAMPTZ,                    -- 吊销（软删语义——保留审计痕迹）
    last_used_at TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_pat_user ON personal_access_tokens (user_id) WHERE revoked_at IS NULL;

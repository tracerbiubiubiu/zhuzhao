-- S3a B3-2 自动分派规则表（03 号附录 A.2 DDL 全文级——五决议既定 §3.2）。
-- 决议① keyword=LIKE 包含匹配（不分词）；③ target_user 须属 target_org（写侧校验）；
-- ④ 无命中保持 open + dispatch_missed 事件；⑤ 失败不回滚创建。
CREATE TABLE IF NOT EXISTS assignment_rules (
    id          BIGSERIAL PRIMARY KEY,
    type_code   VARCHAR(50) NOT NULL,
    priority    INT NOT NULL DEFAULT 100,
    keyword     VARCHAR(100),                     -- LIKE 包含匹配（决议①不分词；NULL=不限）
    target_org  BIGINT NOT NULL REFERENCES organizations(id),
    target_user BIGINT REFERENCES users(id),      -- 决议③须属于 target_org（写侧校验）；NULL=仅分派组织不指定人
    enabled     BOOLEAN NOT NULL DEFAULT TRUE,
    created_by  BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_assignment_rules_type ON assignment_rules (type_code, priority);

-- S10 B10-2 用户自定义标签（10 号附录 A.3 ②——拍板点 6 预确认：默认共享+created_by
-- 留痕，可见性随工单 L2；跨用户标签治理随场景另议不建权限模型）。
-- 通用表（entity_type+entity_id）——phase6 风险事件打标签复用同表零改。
CREATE TABLE IF NOT EXISTS tags (
    id          BIGSERIAL PRIMARY KEY,
    entity_type VARCHAR(30) NOT NULL,    -- ticket（risk 为 phase6 预留值）
    entity_id   BIGINT NOT NULL,
    tag         VARCHAR(50) NOT NULL,
    created_by  BIGINT NOT NULL REFERENCES users(id),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (entity_type, entity_id, tag) -- 同实体同标签唯一（幂等挂载）
);
CREATE INDEX IF NOT EXISTS idx_tags_entity ON tags (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_tags_tag    ON tags (tag);            -- 标签筛选与输入联想

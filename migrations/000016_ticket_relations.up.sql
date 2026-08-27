-- 000016: 工单关联（2a 前移，纯 DB）
-- SSOT: docs/phase2/09-ticket.md §2 工单关联
-- 建立关联时对 target_ticket_id 走 L2/L3 鉴权，防越权关联他人工单

CREATE TABLE IF NOT EXISTS ticket_relations (
    id                BIGSERIAL PRIMARY KEY,
    source_ticket_id  BIGINT NOT NULL REFERENCES tickets(id),
    target_ticket_id  BIGINT NOT NULL REFERENCES tickets(id),
    relation_type     VARCHAR(30) NOT NULL DEFAULT 'related',  -- related/blocks/duplicates/split
    created_by        BIGINT NOT NULL,
    created_at        TIMESTAMPTZ DEFAULT NOW(),
    deleted_at        TIMESTAMPTZ,
    CHECK (source_ticket_id <> target_ticket_id)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_relations_pair ON ticket_relations(source_ticket_id, target_ticket_id, relation_type) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ticket_relations_target ON ticket_relations(target_ticket_id) WHERE deleted_at IS NULL;

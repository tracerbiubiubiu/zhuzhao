-- 000015: 工单模板（2a 前移，纯 DB）
-- SSOT: docs/phase2/09-ticket.md §2 工单模板
-- default_sla_minutes 仅存储，Phase 3 SLA 启用时取用

CREATE TABLE IF NOT EXISTS ticket_templates (
    id                  BIGSERIAL PRIMARY KEY,
    code                VARCHAR(50) NOT NULL,
    name                VARCHAR(200) NOT NULL,
    type_code           VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    default_priority    SMALLINT DEFAULT 3,
    default_fields      JSONB DEFAULT '{}',
    default_sla_minutes INT,
    org_id              BIGINT NOT NULL REFERENCES organizations(id),
    org_path            ltree NOT NULL,
    created_by          BIGINT NOT NULL,
    created_at          TIMESTAMPTZ DEFAULT NOW(),
    updated_at          TIMESTAMPTZ DEFAULT NOW(),
    deleted_at          TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_templates_code ON ticket_templates(code) WHERE deleted_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_ticket_templates_org_path ON ticket_templates USING GIST (org_path);

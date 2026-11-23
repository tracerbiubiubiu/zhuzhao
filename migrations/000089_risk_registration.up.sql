-- 000089: Phase 6 B2 第 6 项（D08/D09）风险登记与核实域
-- DDL SSOT: docs/phase6/risk-module-detail.md §2–4/§7（逻辑模型的最小纵切，
-- 接受/抑制/派单桥等后续项另起编号，不在本迁移）
-- 表族：risk_sources / risk_families / risk_items / risk_source_records /
--       risk_observations / risk_ingest_conflicts / risk_operations / risk_events
-- 菜单种子：risk_manage 目录 + risk_list 页面（仅 GET）+ 三个写按钮（B 案守卫合规）

-- ① 合成来源登记（显式源身份与范围；真实服务凭据接入属 D08 完整范围另批）
CREATE TABLE IF NOT EXISTS risk_sources (
    id                BIGSERIAL PRIMARY KEY,
    source_ref        VARCHAR(100) UNIQUE NOT NULL,
    display_name      VARCHAR(200) NOT NULL,
    fingerprint_scope VARCHAR(100) NOT NULL,
    key_schema_version INT  NOT NULL DEFAULT 1 CHECK (key_schema_version > 0),
    ordered           BOOLEAN NOT NULL DEFAULT TRUE,   -- 序列语义（按 source_sequence 比较）
    mapping_ref       VARCHAR(200) NOT NULL,
    allowed_org_ids   BIGINT[] NOT NULL DEFAULT '{}',  -- 显式范围；空数组 = 未授权任何组织
    default_org_id    BIGINT REFERENCES organizations(id), -- 观察落地的管理组织；必须在 allowed_org_ids 内
    enabled           BOOLEAN NOT NULL DEFAULT FALSE,
    note              TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- ② 稳定漏洞键指纹族（family scope=源 fingerprint_scope；手工源=manual 命名空间）
CREATE TABLE IF NOT EXISTS risk_families (
    id                 BIGSERIAL PRIMARY KEY,
    fingerprint_scope  VARCHAR(100) NOT NULL,
    canonical_key_hash TEXT NOT NULL,                   -- sha256 hex(canonical key)
    key_schema_version INT  NOT NULL DEFAULT 1 CHECK (key_schema_version > 0),
    current_cycle_id   BIGINT,
    revision           BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (fingerprint_scope, canonical_key_hash, key_schema_version)
);

-- ③ 风险周期（每源观察条目一条本地记录；generation/regression_of 为回归语义预留列，
--    首版本轮只写 generation=1，回归判定属后续批次）
CREATE TABLE IF NOT EXISTS risk_items (
    id              BIGSERIAL PRIMARY KEY,
    human_key       VARCHAR(64) UNIQUE NOT NULL,
    family_id       BIGINT NOT NULL REFERENCES risk_families(id),
    generation      INT NOT NULL DEFAULT 1 CHECK (generation >= 1),
    regression_of   BIGINT REFERENCES risk_items(id),
    status          VARCHAR(20) NOT NULL DEFAULT 'new'
                    CHECK (status IN ('new','assessed','remediating','pending_verify','accepted','closed')),
    closure_kind    VARCHAR(20)
                    CHECK (closure_kind IN ('fixed','false_positive','duplicate','transfer','avoid') OR closure_kind IS NULL),
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    org_id          BIGINT NOT NULL REFERENCES organizations(id),
    affected_org_id BIGINT REFERENCES organizations(id),
    owner_user_id   BIGINT,          -- 责任待确认 = NULL（派单准备阻断 owner_unconfirmed）
    level           VARCHAR(10) CHECK (level IN ('urgent','high','medium','low') OR level IS NULL),
    deadline        TIMESTAMPTZ,
    init_deadline   TIMESTAMPTZ,
    treatment       VARCHAR(10) CHECK (treatment IN ('mitigate','accept','transfer','avoid') OR treatment IS NULL),
    accepted_until  TIMESTAMPTZ,
    closed_at       TIMESTAMPTZ,
    archived_at     TIMESTAMPTZ,     -- 查询标签，非业务终态（详设 §3）
    title           VARCHAR(200) NOT NULL,
    description     TEXT,
    system_ref      VARCHAR(200),
    finding_key     VARCHAR(200),    -- 稳定漏洞键（源来源项保留原文；人工源为空）
    asset_provider  VARCHAR(100),    -- 资产键身份原样保留（源来源项；人工源为空）
    asset_external_id VARCHAR(200),
    evidence_refs   JSONB NOT NULL DEFAULT '[]',
    attributes      JSONB NOT NULL DEFAULT '{}',
    created_by      BIGINT NOT NULL, -- 登记人/上报操作者（审计）
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_risk_items_org_status        ON risk_items (org_id, status, id);
CREATE INDEX IF NOT EXISTS idx_risk_items_affected_status   ON risk_items (affected_org_id, status, id);
CREATE INDEX IF NOT EXISTS idx_risk_items_owner_status      ON risk_items (owner_user_id, status, id);
CREATE INDEX IF NOT EXISTS idx_risk_items_level_deadline    ON risk_items (level, deadline, id);
CREATE INDEX IF NOT EXISTS idx_risk_items_family            ON risk_items (family_id);

-- ④ 来源投影（每 (source_ref, source_item_key) 一行镜像；源删除只改可用性）
CREATE TABLE IF NOT EXISTS risk_source_records (
    source_ref      VARCHAR(100) NOT NULL REFERENCES risk_sources(source_ref),
    source_item_key VARCHAR(300) NOT NULL,
    latest_sequence TEXT,
    observed_at     TIMESTAMPTZ,
    content_hash    BYTEA NOT NULL CHECK (octet_length(content_hash) = 32),
    mapping_ref     VARCHAR(200) NOT NULL,
    mirror          JSONB NOT NULL DEFAULT '{}',
    revision        BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (source_ref, source_item_key)
);

-- ⑤ 风险接入台账（先收件后应用；同 (source_ref, event_id) 唯一）
CREATE TABLE IF NOT EXISTS risk_observations (
    ingest_id         VARCHAR(64) PRIMARY KEY,          -- 服务端生成的持久收件身份
    source_ref        VARCHAR(100) NOT NULL REFERENCES risk_sources(source_ref),
    event_id          VARCHAR(200) NOT NULL,
    source_item_key   VARCHAR(300) NOT NULL,
    source_sequence   TEXT,
    observed_at       TIMESTAMPTZ NOT NULL,
    content_hash      BYTEA NOT NULL CHECK (octet_length(content_hash) = 32),
    mapping_ref       VARCHAR(200) NOT NULL,
    kind              VARCHAR(20) NOT NULL DEFAULT 'observed'
                      CHECK (kind IN ('observed','withdrawn','deleted')),
    processing_status VARCHAR(20) NOT NULL DEFAULT 'pending'
                      CHECK (processing_status IN ('pending','applied','held')),
    result_kind       VARCHAR(20) NOT NULL DEFAULT 'accepted'
                      CHECK (result_kind IN ('accepted','duplicate','stale','quarantined','held')),
    risk_id           BIGINT REFERENCES risk_items(id),
    raw_ref           TEXT,                             -- 受控原文引用位（本轮不存原文）
    received_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    safe_reason       TEXT,
    UNIQUE (source_ref, event_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS uq_risk_observations_sequence
    ON risk_observations (source_ref, source_item_key, source_sequence)
    WHERE source_sequence IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_risk_observations_status
    ON risk_observations (processing_status, received_at, ingest_id);

-- ⑥ 接入冲突隔离（原观察与已应用镜像不可变；同冲突重投返回原引用）
CREATE TABLE IF NOT EXISTS risk_ingest_conflicts (
    id                BIGSERIAL PRIMARY KEY,
    original_ingest_id VARCHAR(64) NOT NULL REFERENCES risk_observations(ingest_id),
    source_ref        VARCHAR(100) NOT NULL,
    event_id          VARCHAR(200) NOT NULL,
    source_item_key   VARCHAR(300) NOT NULL,
    source_sequence   TEXT,
    content_hash      BYTEA NOT NULL CHECK (octet_length(content_hash) = 32),
    raw_ref           TEXT,
    conflict_kind     VARCHAR(30) NOT NULL
                      CHECK (conflict_kind IN ('event_hash_mismatch','sequence_hash_mismatch')),
    state             VARCHAR(20) NOT NULL DEFAULT 'open'
                      CHECK (state IN ('open','resolved')),
    received_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    resolved_by       BIGINT,
    resolved_at       TIMESTAMPTZ,
    resolve_reason    TEXT,
    UNIQUE (source_ref, event_id, content_hash)
);
CREATE INDEX IF NOT EXISTS idx_risk_ingest_conflicts_scope
    ON risk_ingest_conflicts (source_ref, state, received_at, id);

-- ⑦ durable 操作收据（同号同参幂等返回原收据，同号异参 409；形态沿 000087 工单回执）
CREATE TABLE IF NOT EXISTS risk_operations (
    actor_id       BIGINT NOT NULL,
    operation_id   VARCHAR(128) NOT NULL CHECK (octet_length(operation_id) BETWEEN 1 AND 128),
    request_hash   BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    command        VARCHAR(30) NOT NULL,                 -- create/assess/assign
    result_status  VARCHAR(20) NOT NULL,
    result_risk_id BIGINT REFERENCES risk_items(id),
    result_revision BIGINT CHECK (result_revision > 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (actor_id, operation_id)
);

-- ⑧ 风险域追加事件（核实/责任/来源应用等处置历史；同事务追加）
CREATE TABLE IF NOT EXISTS risk_events (
    id                 BIGSERIAL PRIMARY KEY,
    risk_id            BIGINT NOT NULL REFERENCES risk_items(id),
    aggregate_revision BIGINT NOT NULL CHECK (aggregate_revision > 0),
    event_kind         VARCHAR(40) NOT NULL,
    ordinal            INT NOT NULL CHECK (ordinal >= 0),
    actor_id           BIGINT NOT NULL,
    occurred_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    operation_id       VARCHAR(128),
    payload            JSONB NOT NULL DEFAULT '{}',
    UNIQUE (risk_id, aggregate_revision, event_kind, ordinal)
);
CREATE INDEX IF NOT EXISTS idx_risk_events_timeline ON risk_events (risk_id, occurred_at, id);

-- ============ 菜单种子（B 案：页面仅 GET，写端点挂按钮行） ============

-- ⑨-a 目录 + 页面
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES
    ('risk_manage', '风险管理', NULL, 1, '/risk', '', 'risk', NULL, 3, true),
    ('risk_list',   '风险列表', NULL, 2, '/risks', 'risk/items/index', 'risk-list', 'risk:read', 1, true)
ON CONFLICT DO NOTHING;

UPDATE menus SET parent_id = (SELECT id FROM menus WHERE code = 'risk_manage')
WHERE code = 'risk_list' AND parent_id IS NULL;

-- ⑨-b 按钮（menu_type=3，risk:manage 码）
INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, v.permission, v.sort_order, true
FROM (VALUES
    ('risk_list', 'risk_create_btn', '风险登记',   'risk:manage', 1),
    ('risk_list', 'risk_assess_btn', '核实评估',   'risk:manage', 3),
    ('risk_list', 'risk_assign_btn', '责任确认',   'risk:manage', 4)
) AS v(parent_code, code, name, permission, sort_order)
JOIN menus p ON p.code = v.parent_code
ON CONFLICT DO NOTHING;

-- ⑨-c menu_apis：页面行仅 GET；写端点挂按钮行
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('risk_list', '/api/v1/risk-items',                      'GET'),
    ('risk_list', '/api/v1/risk-items/:id',                  'GET'),
    ('risk_list', '/api/v1/risk-items/:id/history',          'GET'),
    ('risk_list', '/api/v1/risk-items/:id/asset',            'GET'),
    ('risk_list', '/api/v1/risk-items/ingest-receipts/:id',  'GET'),
    ('risk_list', '/api/v1/risk-items/operations/:id',       'GET')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('risk_create_btn', '/api/v1/risk-items/create',  'POST'),
    ('risk_assess_btn', '/api/v1/risk-items/assess',  'POST'),
    ('risk_assign_btn', '/api/v1/risk-items/assign',  'POST')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code
ON CONFLICT DO NOTHING;

-- ⑨-d 角色绑定：admin/superadmin（000031 惯例）
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r, menus m
WHERE r.code IN ('superadmin', 'admin')
  AND r.deleted_at IS NULL AND m.deleted_at IS NULL
  AND m.code IN (
    'risk_manage', 'risk_list',
    'risk_create_btn', 'risk_assess_btn', 'risk_assign_btn')
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- ⑨-e casbin 派生（沿 000088 形态）
INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, a.api_path, a.api_method
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.deleted_at IS NULL
JOIN menu_apis a ON a.menu_id = m.id
WHERE r.deleted_at IS NULL AND m.code IN (
    'risk_list', 'risk_create_btn', 'risk_assess_btn', 'risk_assign_btn')
ON CONFLICT DO NOTHING;

-- 个人派单只允许选择已配置的整改目标。配置的每次修改应推进 revision。
CREATE TABLE risk_dispatch_targets (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    management_org_id    BIGINT NOT NULL REFERENCES organizations(id),
    target_org_id        BIGINT NOT NULL REFERENCES organizations(id),
    type_code            VARCHAR(50) NOT NULL REFERENCES ticket_types(code),
    approved_scope       TEXT NOT NULL CHECK (approved_scope <> ''),
    revision             BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    enabled              BOOLEAN NOT NULL DEFAULT FALSE,
    UNIQUE (management_org_id, target_org_id, type_code)
);

CREATE TABLE risk_dispatch_plans (
    id                   UUID PRIMARY KEY,
    actor_id             BIGINT NOT NULL CHECK (actor_id > 0),
    plan_hash            CHAR(64) NOT NULL CHECK (plan_hash ~ '^[0-9a-f]{64}$'),
    binding              JSONB NOT NULL CHECK (jsonb_typeof(binding) = 'object'),
    created_at           TIMESTAMPTZ NOT NULL,
    expires_at           TIMESTAMPTZ NOT NULL CHECK (expires_at > created_at),
    UNIQUE (id, actor_id, plan_hash)
);

-- parent 操作先绑定预览和实际操作者，每组的业务提交另带同事务收据。
CREATE TABLE risk_dispatch_operations (
    actor_id             BIGINT NOT NULL,
    operation_id         VARCHAR(128) NOT NULL,
    plan_id              UUID NOT NULL,
    plan_hash            CHAR(64) NOT NULL,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (actor_id, operation_id),
    FOREIGN KEY (actor_id, operation_id) REFERENCES risk_operations(actor_id, operation_id),
    FOREIGN KEY (plan_id, actor_id, plan_hash) REFERENCES risk_dispatch_plans(id, actor_id, plan_hash)
);

CREATE TABLE risk_dispatch_groups (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    finding_key          VARCHAR(200) NOT NULL CHECK (finding_key <> ''),
    owner_user_id        BIGINT NOT NULL CHECK (owner_user_id > 0),
    owner_org_at_dispatch BIGINT,
    management_org_id    BIGINT NOT NULL CHECK (management_org_id > 0),
    target_org_id        BIGINT NOT NULL CHECK (target_org_id > 0),
    type_code            VARCHAR(50) NOT NULL,
    definition_version   BIGINT NOT NULL CHECK (definition_version > 0),
    approved_scope       TEXT NOT NULL CHECK (approved_scope <> ''),
    group_key            CHAR(64) NOT NULL CHECK (group_key ~ '^[0-9a-f]{64}$'),
    ticket_id            BIGINT UNIQUE REFERENCES tickets(id) ON DELETE SET NULL,
    ticket_ref_snapshot  TEXT NOT NULL CHECK (ticket_ref_snapshot <> ''),
    operator_user_id     BIGINT NOT NULL CHECK (operator_user_id > 0),
    business_due_at      TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at             TIMESTAMPTZ,
    CHECK (ticket_id IS NOT NULL OR ended_at IS NOT NULL)
);

-- 目标或发布版本变动不能让同一漏洞、同一责任人另开一张在办票。
CREATE UNIQUE INDEX risk_dispatch_one_live_personal_group
    ON risk_dispatch_groups (finding_key, owner_user_id) WHERE ended_at IS NULL;

CREATE TABLE risk_ticket_links (
    id                   BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    risk_id              BIGINT NOT NULL REFERENCES risk_items(id),
    group_id             BIGINT NOT NULL REFERENCES risk_dispatch_groups(id),
    revision             BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    owner_at_link        BIGINT NOT NULL CHECK (owner_at_link > 0),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    ended_at             TIMESTAMPTZ,
    end_reason           TEXT,
    CHECK ((ended_at IS NULL) = (end_reason IS NULL))
);

CREATE UNIQUE INDEX risk_dispatch_one_active_link
    ON risk_ticket_links (risk_id) WHERE ended_at IS NULL;
CREATE INDEX risk_ticket_links_group ON risk_ticket_links (group_id, id);

CREATE TABLE risk_dispatch_group_receipts (
    actor_id             BIGINT NOT NULL,
    operation_id         VARCHAR(128) NOT NULL,
    group_key            CHAR(64) NOT NULL CHECK (group_key ~ '^[0-9a-f]{64}$'),
    result               JSONB NOT NULL CHECK (jsonb_typeof(result) = 'object'),
    created_at           TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (actor_id, operation_id, group_key),
    FOREIGN KEY (actor_id, operation_id) REFERENCES risk_dispatch_operations(actor_id, operation_id)
);

INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT m.id, 'risk_dispatch_btn', '个人派单', 3, 'risk:manage', 5, true FROM menus m WHERE m.code = 'risk_list';

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT menus.id, paths.path, 'POST'
FROM menus CROSS JOIN (VALUES
    ('/api/v1/risk-items/dispatch-plan'),
    ('/api/v1/risk-items/dispatch'),
    ('/api/v1/risk-items/batch-dispatch')
) AS paths(path)
WHERE menus.code = 'risk_dispatch_btn';

INSERT INTO role_menus (role_id, menu_id)
SELECT roles.id, menus.id FROM roles, menus
WHERE roles.code IN ('superadmin', 'admin') AND roles.deleted_at IS NULL AND menus.code = 'risk_dispatch_btn'
ON CONFLICT (role_id, menu_id) DO NOTHING;

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || roles.code, menu_apis.api_path, menu_apis.api_method
FROM roles
JOIN role_menus ON role_menus.role_id = roles.id
JOIN menus ON menus.id = role_menus.menu_id AND menus.code = 'risk_dispatch_btn'
JOIN menu_apis ON menu_apis.menu_id = menus.id
WHERE NOT EXISTS (
    SELECT 1 FROM casbin_rule
    WHERE ptype = 'p' AND v0 = 'role::' || roles.code
      AND v1 = menu_apis.api_path AND v2 = menu_apis.api_method
);

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, paths.path, 'GET'
FROM menus m CROSS JOIN (VALUES
    ('/api/v1/risk-items/dispatch-operations/:id'),
    ('/api/v1/risk-items/dispatch-statistics')
) AS paths(path)
WHERE m.code = 'risk_list';

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT 'p', 'role::' || roles.code, menu_apis.api_path, 'GET'
FROM roles
JOIN role_menus ON role_menus.role_id = roles.id
JOIN menus ON menus.id = role_menus.menu_id AND menus.code = 'risk_list'
JOIN menu_apis ON menu_apis.menu_id = menus.id AND menu_apis.api_method = 'GET'
WHERE roles.deleted_at IS NULL
  AND menu_apis.api_path IN ('/api/v1/risk-items/dispatch-operations/:id', '/api/v1/risk-items/dispatch-statistics');

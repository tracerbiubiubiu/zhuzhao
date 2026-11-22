-- Plans retain the reviewed type identity even if an unpublished draft is deleted.
CREATE TABLE ticket_definition_publication_plans (
    id BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    actor_id BIGINT NOT NULL CHECK (actor_id > 0),
    type_id BIGINT NOT NULL CHECK (type_id > 0),
    type_code VARCHAR(50) NOT NULL,
    draft_revision BIGINT NOT NULL CHECK (draft_revision > 0),
    expected_published_version BIGINT CHECK (expected_published_version > 0),
    canonical_hash TEXT NOT NULL CHECK (canonical_hash ~ '^[0-9a-f]{64}$'),
    snapshot_hash TEXT NOT NULL CHECK (snapshot_hash ~ '^[0-9a-f]{64}$'),
    reference_revisions JSONB NOT NULL CHECK (jsonb_typeof(reference_revisions) = 'object'),
    warnings JSONB NOT NULL CHECK (jsonb_typeof(warnings) = 'array'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A receipt identifies one committed result, including equivalent publications.
CREATE TABLE ticket_definition_publication_receipts (
    actor_id BIGINT NOT NULL CHECK (actor_id > 0),
    operation_id VARCHAR(128) NOT NULL CHECK (operation_id <> ''),
    request_hash TEXT NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    plan_id BIGINT NOT NULL REFERENCES ticket_definition_publication_plans(id),
    type_code VARCHAR(50) NOT NULL,
    definition_version BIGINT NOT NULL CHECK (definition_version > 0),
    definition_hash TEXT NOT NULL CHECK (definition_hash ~ '^[0-9a-f]{64}$'),
    draft_revision BIGINT NOT NULL CHECK (draft_revision > 0),
    changed BOOLEAN NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (actor_id, operation_id),
    FOREIGN KEY (type_code, definition_version) REFERENCES ticket_type_definitions(type_code, definition_version)
);

-- Store exactly the legacy grants removed below for a clean down migration.
CREATE TABLE ticket_definition_draft_policy_revocations (
    rule_id BIGINT PRIMARY KEY,
    role_id BIGINT NOT NULL,
    subject TEXT NOT NULL,
    v3 TEXT,
    v4 TEXT,
    v5 TEXT
);

INSERT INTO menus (parent_id, menu_type, code, name, path, component, permission, sort_order, icon, visible, is_system)
SELECT parent.id, 3, 'ticket_definition_publish_btn', '发布工单规则', '', '', 'ticket:type:publish', 2, '', TRUE, TRUE
FROM menus parent WHERE parent.code = 'ticket_type_manage' AND parent.deleted_at IS NULL;

INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id
FROM roles r JOIN menus m ON m.code = 'ticket_definition_publish_btn'
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.path, v.method
FROM (VALUES
    ('ticket_definition_publish_btn', '/api/v1/ticket-types/definitions/plan', 'POST'),
    ('ticket_definition_publish_btn', '/api/v1/ticket-types/definitions/publish', 'POST'),
    ('ticket_create_btn', '/api/v1/ticket-types/:code/published', 'GET'),
    ('ticket_type_manage', '/api/v1/ticket-types/:code/published', 'GET'),
    ('ticket_type_write_btn', '/api/v1/ticket-types/:code/fields', 'GET')
) AS v(menu_code, path, method)
JOIN menus m ON m.code = v.menu_code AND m.deleted_at IS NULL;

-- Materialize existing assignments for the new routes. AssignMenus uses the
-- same menu_apis bindings when an administrator changes a role's menus.
INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, a.api_path, a.api_method
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.deleted_at IS NULL
JOIN menu_apis a ON a.menu_id = m.id
WHERE r.deleted_at IS NULL
  AND (m.code = 'ticket_definition_publish_btn'
       OR (m.code IN ('ticket_create_btn', 'ticket_type_manage')
           AND (a.api_path, a.api_method) = ('/api/v1/ticket-types/:code/published', 'GET')))
ON CONFLICT DO NOTHING;

-- The legacy fields endpoint now reads the management draft. Remove the list
-- page's source and the exact derived policy if no remaining menu grants it.
DELETE FROM menu_apis a USING menus m
WHERE a.menu_id = m.id AND m.code = 'ticket_list'
  AND (a.api_path, a.api_method) = ('/api/v1/ticket-types/:code/fields', 'GET');

WITH removed AS (
    DELETE FROM casbin_rule c USING roles r
    WHERE c.ptype = 'p' AND (c.v1, c.v2) = ('/api/v1/ticket-types/:code/fields', 'GET')
      AND c.v0 = 'role::' || r.code AND r.deleted_at IS NULL
      AND EXISTS (
          SELECT 1 FROM role_menus rm
          JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_list' AND m.deleted_at IS NULL
          WHERE rm.role_id = r.id
      )
      AND NOT EXISTS (
          SELECT 1 FROM role_menus remaining
          JOIN menus source ON source.id = remaining.menu_id AND source.deleted_at IS NULL
          JOIN menu_apis a ON a.menu_id = source.id
          WHERE remaining.role_id = r.id AND (a.api_path, a.api_method) = (c.v1, c.v2)
      )
    RETURNING c.id, r.id AS role_id, c.v0, c.v3, c.v4, c.v5
)
INSERT INTO ticket_definition_draft_policy_revocations (rule_id, role_id, subject, v3, v4, v5)
SELECT id, role_id, v0, v3, v4, v5 FROM removed;

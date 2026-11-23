-- B2: per-asset remediation proposals, reviews and independent acceptance.
-- Existing links remain open; published ticket definitions and IAM grants are
-- unchanged. Domain history, receipts and intents join the owning transaction.
ALTER TABLE risk_ticket_links
    ADD COLUMN remediation_state TEXT NOT NULL DEFAULT 'open'
        CHECK (remediation_state IN ('open', 'pending_verify', 'rejected', 'pending_acceptance', 'resolved')),
    ADD COLUMN proposal_version BIGINT NOT NULL DEFAULT 0 CHECK (proposal_version >= 0),
    ADD COLUMN resolution_version BIGINT NOT NULL DEFAULT 0 CHECK (resolution_version >= 0),
    ADD COLUMN resolution TEXT CHECK (resolution IN ('fixed', 'accepted')),
    ADD COLUMN resolved_risk_revision BIGINT CHECK (resolved_risk_revision > 0),
    ADD COLUMN effective_owner_id BIGINT CHECK (effective_owner_id > 0),
    ADD COLUMN effective_at TIMESTAMPTZ,
    ADD CONSTRAINT risk_ticket_links_remediation_resolution CHECK (
        (remediation_state = 'resolved'
            AND resolution IS NOT NULL AND resolved_risk_revision IS NOT NULL
            AND effective_owner_id IS NOT NULL AND effective_at IS NOT NULL)
        OR
        (remediation_state <> 'resolved'
            AND resolution IS NULL AND resolved_risk_revision IS NULL
            AND effective_owner_id IS NULL AND effective_at IS NULL)
    );

CREATE TABLE risk_remediation_submissions (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    link_id             BIGINT NOT NULL REFERENCES risk_ticket_links(id),
    risk_id             BIGINT NOT NULL REFERENCES risk_items(id),
    proposal_version    BIGINT NOT NULL CHECK (proposal_version > 0),
    risk_revision       BIGINT NOT NULL CHECK (risk_revision > 0),
    owner_at_submission BIGINT NOT NULL CHECK (owner_at_submission > 0),
    actor_id            BIGINT NOT NULL CHECK (actor_id > 0),
    resolution          TEXT NOT NULL CHECK (resolution IN ('fixed', 'accept')),
    measures            TEXT NOT NULL CHECK (btrim(measures) <> ''),
    evidence_refs       JSONB NOT NULL CHECK (
        jsonb_typeof(evidence_refs) = 'array' AND evidence_refs <> '[]'::jsonb
    ),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    UNIQUE (link_id, proposal_version)
);

CREATE TABLE risk_remediation_reviews (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    submission_id  BIGINT NOT NULL UNIQUE REFERENCES risk_remediation_submissions(id),
    decision       TEXT NOT NULL CHECK (decision IN ('approve', 'reject')),
    actor_id       BIGINT NOT NULL CHECK (actor_id > 0),
    note           TEXT NOT NULL CHECK (btrim(note) <> ''),
    evidence_refs  JSONB NOT NULL CHECK (jsonb_typeof(evidence_refs) = 'array'),
    risk_revision  BIGINT NOT NULL CHECK (risk_revision > 0),
    link_revision  BIGINT NOT NULL CHECK (link_revision > 0),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE TABLE risk_acceptances (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    risk_id           BIGINT NOT NULL REFERENCES risk_items(id),
    link_id           BIGINT REFERENCES risk_ticket_links(id),
    applicant_id      BIGINT NOT NULL CHECK (applicant_id > 0),
    owner_at_request  BIGINT NOT NULL CHECK (owner_at_request > 0),
    risk_revision     BIGINT NOT NULL CHECK (risk_revision > 0),
    link_revision     BIGINT CHECK (link_revision > 0),
    reason            TEXT NOT NULL CHECK (btrim(reason) <> ''),
    valid_until       TIMESTAMPTZ NOT NULL,
    status            TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'rejected', 'superseded', 'expired')),
    revision          BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0),
    decided_by        BIGINT CHECK (decided_by > 0),
    decided_at        TIMESTAMPTZ,
    decided_reason    TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    CHECK (applicant_id = owner_at_request),
    CHECK ((link_id IS NULL) = (link_revision IS NULL)),
    CHECK (status <> 'pending'
        OR (decided_by IS NULL AND decided_at IS NULL AND decided_reason IS NULL)),
    CHECK (status NOT IN ('approved', 'rejected')
        OR (decided_by IS NOT NULL AND decided_at IS NOT NULL
            AND decided_reason IS NOT NULL AND btrim(decided_reason) <> '')),
    CHECK (status <> 'approved' OR decided_by <> applicant_id)
);
CREATE UNIQUE INDEX risk_acceptances_one_pending
    ON risk_acceptances (risk_id) WHERE status = 'pending';
CREATE INDEX risk_acceptances_expiry
    ON risk_acceptances (valid_until, id) WHERE status = 'approved';

CREATE TABLE risk_remediation_receipts (
    actor_id      BIGINT NOT NULL CHECK (actor_id > 0),
    operation_id  VARCHAR(128) NOT NULL,
    ticket_id     BIGINT, -- immutable reference snapshot; survives ticket deletion
    result        JSONB NOT NULL CHECK (jsonb_typeof(result) = 'object'),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (actor_id, operation_id),
    FOREIGN KEY (actor_id, operation_id) REFERENCES risk_operations(actor_id, operation_id)
);

CREATE TABLE risk_remediation_intents (
    id                  BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    risk_id             BIGINT NOT NULL REFERENCES risk_items(id),
    link_id             BIGINT REFERENCES risk_ticket_links(id),
    ticket_id           BIGINT, -- immutable reference snapshot; no live-ticket FK
    ticket_revision     BIGINT CHECK (ticket_revision > 0),
    resolution_version  BIGINT NOT NULL DEFAULT 0 CHECK (resolution_version >= 0),
    aggregate_revision  BIGINT NOT NULL CHECK (aggregate_revision > 0),
    event_kind          TEXT NOT NULL CHECK (btrim(event_kind) <> ''),
    operation_id        VARCHAR(128) NOT NULL CHECK (octet_length(operation_id) BETWEEN 1 AND 128),
    actor_id            BIGINT NOT NULL CHECK (actor_id > 0),
    created_at          TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    payload             JSONB NOT NULL CHECK (jsonb_typeof(payload) = 'object'),
    UNIQUE (risk_id, aggregate_revision, event_kind)
);

INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT p.id, v.code, v.name, 3, 'risk:manage', v.sort_order, true
FROM (VALUES
    ('risk_remediation_finish_btn', '提交风险整改', 6),
    ('risk_remediation_review_btn', '逐项复测', 7),
    ('risk_remediation_accept_request_btn', '申请风险接受', 8),
    ('risk_remediation_accept_decision_btn', '审批风险接受', 9),
    ('risk_remediation_assign_handler_btn', '分派整改办理人', 10),
    ('risk_remediation_accept_assignment_btn', '接单办理风险整改', 11)
) AS v(code, name, sort_order)
JOIN menus p ON p.code = 'risk_list';

INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r, menus m
WHERE r.deleted_at IS NULL AND r.code IN ('admin', 'superadmin')
  AND m.code IN ('risk_remediation_finish_btn', 'risk_remediation_review_btn',
      'risk_remediation_accept_request_btn', 'risk_remediation_accept_decision_btn',
      'risk_remediation_assign_handler_btn', 'risk_remediation_accept_assignment_btn')
ON CONFLICT (role_id, menu_id) DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.path, v.method FROM menus m
JOIN (VALUES
    ('risk_list', '/api/v1/risk-items/remediation-context/:id', 'GET'),
    ('risk_list', '/api/v1/risk-items/remediation-operations/:id', 'GET'),
    ('risk_remediation_finish_btn', '/api/v1/risk-items/remediation-finish', 'POST'),
    ('risk_remediation_review_btn', '/api/v1/risk-items/remediation-review', 'POST'),
    ('risk_remediation_accept_request_btn', '/api/v1/risk-items/remediation-accept-request', 'POST'),
    ('risk_remediation_accept_decision_btn', '/api/v1/risk-items/remediation-accept-decision', 'POST'),
    ('risk_remediation_assign_handler_btn', '/api/v1/risk-items/remediation-assign-handler', 'POST'),
    ('risk_remediation_accept_assignment_btn', '/api/v1/risk-items/remediation-accept-assignment', 'POST')
) AS v(code, path, method) ON v.code = m.code;

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, a.api_path, a.api_method
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menu_apis a ON a.menu_id = rm.menu_id
WHERE r.deleted_at IS NULL
  AND a.api_path IN ('/api/v1/risk-items/remediation-context/:id', '/api/v1/risk-items/remediation-operations/:id',
      '/api/v1/risk-items/remediation-finish', '/api/v1/risk-items/remediation-review',
      '/api/v1/risk-items/remediation-accept-request', '/api/v1/risk-items/remediation-accept-decision',
      '/api/v1/risk-items/remediation-assign-handler', '/api/v1/risk-items/remediation-accept-assignment');

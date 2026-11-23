-- Preserve dispatch-time identity while the live uniqueness key follows the reviewer.
ALTER TABLE risk_dispatch_groups
    ADD COLUMN owner_at_dispatch BIGINT,
    ADD COLUMN retest_group_key CHAR(64),
    ADD CONSTRAINT risk_dispatch_retest_binding_shape CHECK (
        (owner_at_dispatch IS NULL AND retest_group_key IS NULL)
        OR (owner_at_dispatch IS NOT NULL AND owner_at_dispatch > 0
            AND retest_group_key IS NOT NULL AND retest_group_key ~ '^[0-9a-f]{64}$')
    );

ALTER TABLE tickets ADD COLUMN risk_retest_reviewer_id BIGINT CHECK (risk_retest_reviewer_id > 0);

-- Keep the business revision under the same trigger used by ordinary writes.
CREATE OR REPLACE FUNCTION advance_ticket_business_revision() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.type_code, NEW.title, NEW.description, NEW.priority, NEW.status,
           NEW.created_by, NEW.assigned_to, NEW.org_id, NEW.org_path, NEW.custom_data,
           NEW.escalation_supervisor_id, NEW.risk_retest_reviewer_id)
       IS DISTINCT FROM
       ROW(OLD.type_code, OLD.title, OLD.description, OLD.priority, OLD.status,
           OLD.created_by, OLD.assigned_to, OLD.org_id, OLD.org_path, OLD.custom_data,
           OLD.escalation_supervisor_id, OLD.risk_retest_reviewer_id)
       OR (OLD.definition_version IS NOT NULL AND
           NEW.definition_version IS DISTINCT FROM OLD.definition_version) THEN
        NEW.revision := OLD.revision + 1;
    ELSE
        NEW.revision := OLD.revision;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TABLE risk_retest_handoff_items (
    link_id                  BIGINT NOT NULL REFERENCES risk_ticket_links(id),
    link_revision            BIGINT NOT NULL CHECK (link_revision > 1),
    risk_revision            BIGINT NOT NULL CHECK (risk_revision > 1),
    submission_id            BIGINT REFERENCES risk_remediation_submissions(id),
    from_owner_id            BIGINT NOT NULL CHECK (from_owner_id > 0),
    from_group_owner_id      BIGINT NOT NULL CHECK (from_group_owner_id > 0),
    successor_user_id        BIGINT NOT NULL CHECK (successor_user_id > 0),
    actor_id                 BIGINT NOT NULL CHECK (actor_id > 0),
    operation_id             VARCHAR(128) NOT NULL,
    reason                   TEXT NOT NULL CHECK (octet_length(reason) BETWEEN 1 AND 2000 AND btrim(reason) <> ''),
    created_at               TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (link_id, link_revision),
    FOREIGN KEY (actor_id, operation_id) REFERENCES risk_remediation_receipts(actor_id, operation_id)
        DEFERRABLE INITIALLY DEFERRED
);

INSERT INTO menus (parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT id, 'risk_remediation_handoff_btn', '交接风险复测', 3, 'risk:manage', 12, true
FROM menus WHERE code = 'risk_list';

INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r, menus m
WHERE r.deleted_at IS NULL AND r.code IN ('admin', 'superadmin')
  AND m.code = 'risk_remediation_handoff_btn';

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT id, '/api/v1/risk-items/remediation-handoff', 'POST'
FROM menus WHERE code = 'risk_remediation_handoff_btn';

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, a.api_path, a.api_method
FROM roles r JOIN role_menus rm ON rm.role_id = r.id
JOIN menu_apis a ON a.menu_id = rm.menu_id
WHERE r.deleted_at IS NULL AND a.api_path = '/api/v1/risk-items/remediation-handoff';

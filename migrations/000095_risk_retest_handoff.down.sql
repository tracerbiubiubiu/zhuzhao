-- The previous release cannot honor successors or preserve their full audit.
LOCK TABLE risk_dispatch_groups, tickets, risk_retest_handoff_items IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT 1 FROM risk_retest_handoff_items)
        OR EXISTS (SELECT 1 FROM risk_dispatch_groups WHERE owner_at_dispatch IS NOT NULL)
        OR EXISTS (SELECT 1 FROM tickets WHERE risk_retest_reviewer_id IS NOT NULL) THEN
        RAISE EXCEPTION 'risk retest handoff facts exist; retain history and use a compatible forward migration';
    END IF;
END $$;

DELETE FROM casbin_rule WHERE ptype = 'p' AND v1 = '/api/v1/risk-items/remediation-handoff' AND v2 = 'POST';
DELETE FROM menu_apis WHERE api_path = '/api/v1/risk-items/remediation-handoff' AND api_method = 'POST';
DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE code = 'risk_remediation_handoff_btn');
DELETE FROM menus WHERE code = 'risk_remediation_handoff_btn';

DROP TABLE risk_retest_handoff_items;
ALTER TABLE risk_dispatch_groups DROP CONSTRAINT risk_dispatch_retest_binding_shape,
    DROP COLUMN owner_at_dispatch, DROP COLUMN retest_group_key;

CREATE OR REPLACE FUNCTION advance_ticket_business_revision() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.type_code, NEW.title, NEW.description, NEW.priority, NEW.status,
           NEW.created_by, NEW.assigned_to, NEW.org_id, NEW.org_path, NEW.custom_data,
           NEW.escalation_supervisor_id)
       IS DISTINCT FROM
       ROW(OLD.type_code, OLD.title, OLD.description, OLD.priority, OLD.status,
           OLD.created_by, OLD.assigned_to, OLD.org_id, OLD.org_path, OLD.custom_data,
           OLD.escalation_supervisor_id)
       OR (OLD.definition_version IS NOT NULL AND
           NEW.definition_version IS DISTINCT FROM OLD.definition_version) THEN
        NEW.revision := OLD.revision + 1;
    ELSE
        NEW.revision := OLD.revision;
    END IF;
    RETURN NEW;
END;
$$;

ALTER TABLE tickets DROP COLUMN risk_retest_reviewer_id;

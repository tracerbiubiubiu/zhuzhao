-- A pre-existing state of this name belongs to the old graph, including
-- immutable versions which are no longer the current published pointer.
-- Fence configuration/publication and ticket writers before testing for it.
LOCK TABLE ticket_types, ticket_type_definitions IN SHARE MODE;
LOCK TABLE tickets IN ACCESS EXCLUSIVE MODE;
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM tickets WHERE status = 'cancelled') OR EXISTS (
        WITH graphs(states, edges, meta) AS (
            SELECT states, transitions, transition_meta FROM ticket_types
            UNION ALL
            SELECT snapshot #> '{type,states}', snapshot #> '{type,transitions}',
                   snapshot #> '{type,transition_meta}' FROM ticket_type_definitions
        )
        SELECT 1 FROM graphs
        WHERE states ? 'cancelled' OR edges ? 'cancelled'
           OR edges @? '$.*[*] ? (@ == "cancelled")'
           OR EXISTS (
               SELECT 1 FROM jsonb_object_keys(
                   CASE WHEN jsonb_typeof(meta) = 'object' THEN meta ELSE '{}'::jsonb END
               ) AS keys(key)
               WHERE key = 'cancelled' OR key LIKE 'cancelled>%' OR key LIKE '%>cancelled'
           )
    ) THEN
        RAISE EXCEPTION 'legacy cancelled ticket state exists; resolve the conflicting state before migration 000106';
    END IF;
END;
$$;

-- Only a trusted new open, unassigned create explicitly asserts this fact.
-- Existing tickets and older writers cannot certify their previous history.
ALTER TABLE tickets ADD COLUMN withdrawal_eligible BOOLEAN NOT NULL DEFAULT false;
CREATE FUNCTION track_ticket_withdrawal_eligibility() RETURNS TRIGGER LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        NEW.withdrawal_eligible := OLD.withdrawal_eligible AND NEW.withdrawal_eligible;
    END IF;
    NEW.withdrawal_eligible := NEW.withdrawal_eligible AND NEW.status='open' AND NEW.assigned_to IS NULL;
    RETURN NEW;
END;
$$;
CREATE TRIGGER tickets_withdrawal_eligibility BEFORE INSERT OR UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION track_ticket_withdrawal_eligibility();

CREATE TABLE ticket_handling_receipts (
    actor_id BIGINT NOT NULL,
    operation_key TEXT NOT NULL CHECK (octet_length(operation_key) BETWEEN 1 AND 128),
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash)=32),
    ticket_id BIGINT NOT NULL,
    result_assignee BIGINT,
    result_status VARCHAR(20) NOT NULL,
    result_revision BIGINT NOT NULL CHECK (result_revision>0),
    PRIMARY KEY(actor_id,operation_key)
);

INSERT INTO menus(parent_id,menu_type,code,name,path,component,permission,sort_order,icon,visible,is_system)
SELECT m.id,3,'ticket_' || v.action || '_btn',v.name,'','','ticket:' || v.action,20,'',true,true
FROM menus m CROSS JOIN (VALUES ('transfer','转办工单'),('withdraw','撤回工单')) v(action,name)
WHERE m.code='ticket_list' AND m.deleted_at IS NULL;
INSERT INTO menu_apis(menu_id,api_path,api_method)
SELECT id,'/api/v1/tickets/' || split_part(permission,':',2),'POST'
FROM menus WHERE code IN ('ticket_transfer_btn','ticket_withdraw_btn') AND deleted_at IS NULL;
INSERT INTO role_menus(role_id,menu_id)
SELECT r.id,m.id FROM roles r CROSS JOIN menus m
WHERE r.code IN ('admin','superadmin') AND r.deleted_at IS NULL
  AND m.code IN ('ticket_transfer_btn','ticket_withdraw_btn') AND m.deleted_at IS NULL;
INSERT INTO casbin_rule(ptype,v0,v1,v2)
SELECT 'p','role::' || r.code,a.api_path,a.api_method
FROM roles r JOIN role_menus rm ON rm.role_id=r.id JOIN menus m ON m.id=rm.menu_id
JOIN menu_apis a ON a.menu_id=m.id WHERE m.code IN ('ticket_transfer_btn','ticket_withdraw_btn')
ON CONFLICT DO NOTHING;

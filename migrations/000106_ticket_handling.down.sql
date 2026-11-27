-- Removing evidence of previous assignments can only make old tickets ineligible
-- after re-upgrade. A successful command receipt must survive a lost response.
-- The previous application has no dedicated creator withdrawal guard.
LOCK TABLE ticket_types, ticket_type_definitions IN SHARE MODE;
LOCK TABLE tickets, ticket_handling_receipts IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM ticket_handling_receipts) THEN
        RAISE EXCEPTION 'retain ticket handling receipts before rollback';
    END IF;
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
        RAISE EXCEPTION 'retain cancelled ticket definitions and instances before rollback';
    END IF;
END $$;
DELETE FROM casbin_rule WHERE ptype='p' AND v2='POST' AND v1 IN ('/api/v1/tickets/transfer','/api/v1/tickets/withdraw');
DELETE FROM role_menus WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('ticket_transfer_btn','ticket_withdraw_btn'));
DELETE FROM menu_apis WHERE menu_id IN (SELECT id FROM menus WHERE code IN ('ticket_transfer_btn','ticket_withdraw_btn'));
DELETE FROM menus WHERE code IN ('ticket_transfer_btn','ticket_withdraw_btn');
DROP TABLE ticket_handling_receipts;
DROP TRIGGER tickets_withdrawal_eligibility ON tickets;
DROP FUNCTION track_ticket_withdrawal_eligibility();
ALTER TABLE tickets DROP COLUMN withdrawal_eligible;

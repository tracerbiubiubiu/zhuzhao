-- Removing evidence of previous assignments can only make old tickets ineligible
-- after re-upgrade. A successful command receipt must survive a lost response.
DO $$ BEGIN
    IF EXISTS(SELECT 1 FROM ticket_handling_receipts) THEN
        RAISE EXCEPTION 'retain ticket handling receipts before rollback';
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

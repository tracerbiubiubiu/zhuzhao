-- Stop D02 writes before rollback. Preserve original receipts after activation.
DO $$
BEGIN
    LOCK TABLE tickets, ticket_evidence_transition_receipts IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM ticket_evidence_transition_receipts) THEN
        RAISE EXCEPTION 'D02 transition receipts exist; retain receipts and revision before rollback';
    END IF;

    DROP TABLE ticket_evidence_transition_receipts;
    DROP TRIGGER tickets_business_revision ON tickets;
    DROP FUNCTION advance_ticket_business_revision();
    ALTER TABLE tickets DROP COLUMN revision;
END;
$$;

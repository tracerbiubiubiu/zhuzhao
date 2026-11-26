DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM command_receipts) OR EXISTS (SELECT 1 FROM activelist_source_inbox)
        OR EXISTS (SELECT 1 FROM b3_grants) THEN
        RAISE EXCEPTION 'B3 durable state exists; disable execution and retain the receipt schema';
    END IF;
END
$$;

DROP TABLE b3_grants;
DROP FUNCTION b3_freeze_grant_revision();
DROP TABLE activelist_source_inbox;
DROP TABLE business_intent_outbox;
DROP TABLE command_receipts;

LOCK TABLE iam_command_receipts, iam_audit_facts, iam_audit_deliveries IN ACCESS EXCLUSIVE MODE;
DO $$ BEGIN
    IF EXISTS (SELECT FROM iam_audit_facts) OR EXISTS (SELECT FROM iam_command_receipts) THEN
        RAISE EXCEPTION 'IAM audit history requires preservation before downgrade';
    END IF;
END $$;
DROP TABLE iam_command_receipts, iam_audit_deliveries, iam_audit_facts;
DROP FUNCTION iam_audit_fact_immutable();

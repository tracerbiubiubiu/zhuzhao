-- Stop reminder writes before rollback. Reminder facts (dispatch records,
-- delivery receipts, suppression reasons) are not reconstructible from other
-- tables, so a rollback with live rows is refused — same contract as the
-- evidence transition receipts (000087).
DO $$
BEGIN
    LOCK TABLE riskreminder_intents, riskreminder_assignments IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM riskreminder_intents)
       OR EXISTS (SELECT 1 FROM riskreminder_assignments) THEN
        RAISE EXCEPTION 'riskreminder facts exist; retain assignments and intents before rollback';
    END IF;

    DROP TABLE riskreminder_intents;
    DROP TABLE riskreminder_assignments;
END;
$$;

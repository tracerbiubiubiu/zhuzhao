DO $$
BEGIN
    LOCK TABLE risk_ingest_batches IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM risk_ingest_batches) THEN
        RAISE EXCEPTION 'Durable risk ingest batches exist; retain the receipt schema';
    END IF;
END
$$;

DROP INDEX idx_risk_observations_applied_risk;
DROP TABLE risk_ingest_batches;

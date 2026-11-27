CREATE TABLE risk_ingest_batches (
    source_ref VARCHAR(100) NOT NULL REFERENCES risk_sources(source_ref),
    receipt_key TEXT NOT NULL CHECK (octet_length(receipt_key) BETWEEN 1 AND 131),
    batch_receipt UUID NOT NULL UNIQUE,
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    org_id BIGINT NOT NULL REFERENCES organizations(id),
    mapping_ref VARCHAR(200) NOT NULL,
    fingerprint_scope VARCHAR(100) NOT NULL,
    key_schema_version INT NOT NULL CHECK (key_schema_version > 0),
    ordered BOOLEAN NOT NULL,
    observations JSONB NOT NULL CHECK (jsonb_typeof(observations) = 'array'
                                      AND jsonb_array_length(observations) BETWEEN 1 AND 100),
    result JSONB CHECK (jsonb_typeof(result) = 'object'),
    received_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    last_attempt_at TIMESTAMPTZ,
    PRIMARY KEY (source_ref, receipt_key)
);

CREATE INDEX idx_risk_ingest_batches_pending
    ON risk_ingest_batches(source_ref, last_attempt_at NULLS FIRST, received_at, batch_receipt)
    WHERE result IS NULL;

CREATE INDEX idx_risk_observations_applied_risk ON risk_observations(risk_id)
    WHERE risk_id IS NOT NULL AND processing_status = 'applied';

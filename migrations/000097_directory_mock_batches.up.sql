-- D06 first slice uses a synthetic projection separate from IAM organizations.
CREATE TABLE directory_mock_scopes (
    scope_ref        TEXT PRIMARY KEY,
    success_baseline TEXT NOT NULL DEFAULT '',
    success_cursor   TEXT NOT NULL DEFAULT ''
);

CREATE TABLE directory_mock_departments (
    scope_ref       TEXT NOT NULL REFERENCES directory_mock_scopes(scope_ref),
    external_id     TEXT NOT NULL,
    display_name    TEXT NOT NULL,
    source_revision BIGINT NOT NULL CHECK (source_revision > 0),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (scope_ref, external_id)
);

CREATE TABLE directory_mock_runs (
    scope_ref  TEXT NOT NULL REFERENCES directory_mock_scopes(scope_ref),
    batch_id   TEXT NOT NULL,
    run_id     UUID NOT NULL UNIQUE,
    input_hash TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    receipt    JSONB NOT NULL CHECK (jsonb_typeof(receipt) = 'object'),
    created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (scope_ref, batch_id)
);

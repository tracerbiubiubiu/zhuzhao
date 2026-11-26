-- Explicit bindings for one synthetic source. Existing department batches keep
-- their own projection and watermark.
CREATE TABLE directory_mock_employee_bindings (
    scope_ref           TEXT NOT NULL REFERENCES directory_mock_scopes(scope_ref),
    external_id         TEXT NOT NULL,
    identity_generation BIGINT NOT NULL CHECK (identity_generation > 0),
    user_id             BIGINT NOT NULL UNIQUE REFERENCES users(id),
    source_revision     BIGINT NOT NULL CHECK (source_revision > 0),
    PRIMARY KEY (scope_ref, external_id)
);

CREATE TABLE directory_mock_department_bindings (
    scope_ref   TEXT NOT NULL,
    external_id TEXT NOT NULL,
    org_id      BIGINT NOT NULL UNIQUE REFERENCES organizations(id),
    PRIMARY KEY (scope_ref, external_id),
    FOREIGN KEY (scope_ref, external_id) REFERENCES directory_mock_departments(scope_ref, external_id)
);

CREATE TABLE directory_mock_transfer_runs (
    scope_ref    TEXT NOT NULL REFERENCES directory_mock_scopes(scope_ref),
    operation_id TEXT NOT NULL,
    run_id       UUID NOT NULL UNIQUE,
    input_hash   TEXT NOT NULL CHECK (input_hash ~ '^[0-9a-f]{64}$'),
    receipt      JSONB NOT NULL CHECK (jsonb_typeof(receipt) = 'object'),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
    PRIMARY KEY (scope_ref, operation_id)
);

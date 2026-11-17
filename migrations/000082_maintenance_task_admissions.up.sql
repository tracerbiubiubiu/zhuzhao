-- Narrow authorization facts for audit_archive only. Existing task/submission
-- rows are deliberately NOT backfilled into authority. No business receipt or
-- external effect outcome is introduced by this migration.
CREATE TABLE maintenance_task_admissions (
    task_id VARCHAR(64) PRIMARY KEY,
    action VARCHAR(50) NOT NULL CHECK (action = 'audit_archive'),
    params TEXT NOT NULL,
    origin VARCHAR(16) NOT NULL CHECK (origin IN ('manual', 'fixed_job')),
    principal_user_id BIGINT NOT NULL DEFAULT 0,
    job_id TEXT NOT NULL DEFAULT '',
    definition_sha256 TEXT NOT NULL DEFAULT '',
    retention_days INTEGER NOT NULL CHECK (retention_days > 0),
    approval_id TEXT NOT NULL DEFAULT '',
    reason TEXT NOT NULL,
    valid_from TIMESTAMPTZ,
    valid_until TIMESTAMPTZ,
    request_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((origin='manual' AND principal_user_id>0) OR
           (origin='fixed_job' AND principal_user_id=0 AND job_id<>'' AND definition_sha256<>''))
);
COMMENT ON TABLE maintenance_task_admissions IS 'Immutable maintenance authorization facts; not execution success receipts. Historical rows and body actor cannot manufacture authority.';

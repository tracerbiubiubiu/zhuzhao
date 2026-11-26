CREATE TABLE iam_audit_facts (
    record_id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    event_name text NOT NULL CHECK (event_name IN ('iam.user.roles.set', 'iam.role.menus.set')),
    occurred_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    actor_id bigint NOT NULL,
    target_id bigint NOT NULL,
    added_ids bigint[] NOT NULL,
    removed_ids bigint[] NOT NULL
);
CREATE FUNCTION iam_audit_fact_immutable() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    RAISE EXCEPTION 'IAM audit facts are immutable';
END $$;
CREATE TRIGGER iam_audit_fact_immutable BEFORE UPDATE OR DELETE OR TRUNCATE ON iam_audit_facts
    FOR EACH STATEMENT EXECUTE FUNCTION iam_audit_fact_immutable();

CREATE TABLE iam_audit_deliveries (
    record_id uuid PRIMARY KEY REFERENCES iam_audit_facts(record_id),
    collector_key text NOT NULL,
    available_at timestamptz NOT NULL DEFAULT now(),
    fence bigint NOT NULL DEFAULT 0,
    delivered_at timestamptz
);
CREATE INDEX iam_audit_delivery_due ON iam_audit_deliveries (collector_key, available_at, record_id)
    WHERE delivered_at IS NULL;

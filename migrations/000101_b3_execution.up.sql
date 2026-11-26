CREATE TABLE command_receipts (
    operation_id text PRIMARY KEY,
    task_id text NOT NULL UNIQUE,
    action text NOT NULL,
    ticket_id bigint,
    ticket_revision bigint,
    definition_hash text,
    node text,
    action_index integer,
    actor_id bigint,
    transition_roles text[] NOT NULL,
    payload jsonb NOT NULL,
    status text NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'succeeded', 'held', 'failed_no_effect')),
    result jsonb,
    reason text,
    execution_generation bigint NOT NULL DEFAULT 0,
    execution_until timestamptz,
    remote_started boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    completed_at timestamptz,
    UNIQUE (ticket_id, ticket_revision, action_index),
    CHECK ((ticket_id IS NULL AND ticket_revision IS NULL AND definition_hash IS NULL
        AND node IS NULL AND action_index IS NULL AND actor_id IS NULL)
        OR (ticket_id IS NOT NULL AND ticket_revision > 0 AND definition_hash IS NOT NULL
        AND node IS NOT NULL AND action_index >= 0 AND actor_id IS NOT NULL)),
    CHECK ((status = 'pending' AND completed_at IS NULL)
        OR (status <> 'pending' AND completed_at IS NOT NULL)),
    CHECK ((status = 'succeeded') = (result IS NOT NULL))
);

CREATE TABLE business_intent_outbox (
    operation_id text PRIMARY KEY REFERENCES command_receipts (operation_id),
    next_attempt_at timestamptz NOT NULL DEFAULT now(),
    attempts integer NOT NULL DEFAULT 0,
    lease_until timestamptz,
    lease_generation bigint NOT NULL DEFAULT 0,
    accepted_at timestamptz
);

CREATE INDEX business_intent_outbox_due
    ON business_intent_outbox (next_attempt_at, operation_id) WHERE accepted_at IS NULL;

CREATE TABLE activelist_source_inbox (
    source text NOT NULL,
    event_id text NOT NULL,
    stream_id text NOT NULL,
    cursor bigint NOT NULL,
    payload_hash text NOT NULL,
    event jsonb NOT NULL,
    stage_commit_ref text NOT NULL UNIQUE,
    received_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (source, event_id),
    UNIQUE (source, stream_id, cursor)
);

CREATE TABLE b3_grants (
    grant_ref text PRIMARY KEY,
    revision bigint NOT NULL CHECK (revision > 0),
    subject_type text NOT NULL CHECK (subject_type IN ('human', 'service')),
    subject_id text NOT NULL,
    type_name text NOT NULL,
    generation bigint NOT NULL CHECK (generation > 0),
    document_id bigint CHECK (document_id > 0),
    read_fields text[] NOT NULL DEFAULT '{}',
    write_fields text[] NOT NULL DEFAULT '{}',
    enabled boolean NOT NULL DEFAULT false,
    CHECK (subject_type <> 'human' OR document_id IS NOT NULL)
);

CREATE FUNCTION b3_freeze_grant_revision() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.revision < OLD.revision OR (NEW.revision = OLD.revision AND
        (NEW.grant_ref, NEW.subject_type, NEW.subject_id, NEW.type_name, NEW.generation,
         NEW.document_id, NEW.read_fields, NEW.write_fields)
        IS DISTINCT FROM
        (OLD.grant_ref, OLD.subject_type, OLD.subject_id, OLD.type_name, OLD.generation,
         OLD.document_id, OLD.read_fields, OLD.write_fields))
        OR (NEW.revision = OLD.revision AND NEW.enabled AND NOT OLD.enabled) THEN
        RAISE EXCEPTION 'B3 grant changes require a new revision';
    END IF;
    RETURN NEW;
END
$$;
CREATE TRIGGER b3_freeze_grant_revision BEFORE UPDATE ON b3_grants
    FOR EACH ROW EXECUTE FUNCTION b3_freeze_grant_revision();

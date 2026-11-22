-- D02 business revision starts at 1 for both existing tickets and new creates.
ALTER TABLE tickets
    ADD COLUMN revision BIGINT NOT NULL DEFAULT 1 CHECK (revision > 0);

-- A single authority covers legacy writers too. The state-entry clock, SLA,
-- updated_at and the generated human key are auxiliary writes. The first
-- trusted legacy binding fills a missing reference without an instance edit.
CREATE FUNCTION advance_ticket_business_revision() RETURNS TRIGGER
LANGUAGE plpgsql AS $$
BEGIN
    IF ROW(NEW.type_code, NEW.title, NEW.description, NEW.priority, NEW.status,
           NEW.created_by, NEW.assigned_to, NEW.org_id, NEW.org_path, NEW.custom_data,
           NEW.escalation_supervisor_id)
       IS DISTINCT FROM
       ROW(OLD.type_code, OLD.title, OLD.description, OLD.priority, OLD.status,
           OLD.created_by, OLD.assigned_to, OLD.org_id, OLD.org_path, OLD.custom_data,
           OLD.escalation_supervisor_id)
       OR (OLD.definition_version IS NOT NULL AND
           NEW.definition_version IS DISTINCT FROM OLD.definition_version) THEN
        NEW.revision := OLD.revision + 1;
    ELSE
        NEW.revision := OLD.revision;
    END IF;
    RETURN NEW;
END;
$$;

CREATE TRIGGER tickets_business_revision
    BEFORE UPDATE ON tickets
    FOR EACH ROW EXECUTE FUNCTION advance_ticket_business_revision();

-- The key belongs to the actor within the transition command. Ticket identity
-- and the result's original business coordinates are sufficient for replay.
-- Deletion detaches the receipt while retaining the key and request digest.
-- Current ticket authorization is required before any receipt read.
CREATE TABLE ticket_evidence_transition_receipts (
    ticket_id BIGINT REFERENCES tickets(id) ON DELETE SET NULL,
    actor_id BIGINT NOT NULL,
    idempotency_key TEXT NOT NULL CHECK (octet_length(idempotency_key) BETWEEN 1 AND 128),
    request_hash BYTEA NOT NULL CHECK (octet_length(request_hash) = 32),
    result_status VARCHAR(20) NOT NULL,
    result_revision BIGINT NOT NULL CHECK (result_revision > 0),
    result_definition_version BIGINT NOT NULL CHECK (result_definition_version > 0),
    result_status_changed_at TIMESTAMPTZ,
    PRIMARY KEY (actor_id, idempotency_key)
);

CREATE INDEX idx_ticket_evidence_transition_receipts_ticket_id
    ON ticket_evidence_transition_receipts (ticket_id);

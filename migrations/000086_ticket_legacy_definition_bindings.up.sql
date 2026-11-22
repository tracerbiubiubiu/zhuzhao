-- D01: retain the approved mapping and evidence digests for each completed
-- legacy bind. A ticket may still be physically deleted through the usual path.
CREATE TABLE ticket_legacy_definition_bindings (
 ticket_id BIGINT PRIMARY KEY CHECK (ticket_id > 0),
 type_code VARCHAR(50) NOT NULL,
 original_created_by BIGINT NOT NULL CHECK (original_created_by > 0),
 original_created_at TIMESTAMPTZ NOT NULL,
 expected_status VARCHAR(20) NOT NULL CHECK (expected_status <> ''),
 definition_version BIGINT NOT NULL,
 definition_hash TEXT NOT NULL CHECK (definition_hash ~ '^[0-9a-f]{64}$'),
 configuration_evidence_sha256 TEXT NOT NULL CHECK (configuration_evidence_sha256 ~ '^[0-9a-f]{64}$'),
 mapping_evidence_sha256 TEXT NOT NULL CHECK (mapping_evidence_sha256 ~ '^[0-9a-f]{64}$'),
 approval_sha256 TEXT NOT NULL CHECK (approval_sha256 ~ '^[0-9a-f]{64}$'),
 ticket_evidence_sha256 TEXT NOT NULL CHECK (ticket_evidence_sha256 ~ '^[0-9a-f]{64}$'),
 bound_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
 FOREIGN KEY (type_code, definition_version) REFERENCES ticket_type_definitions(type_code, definition_version)
);

CREATE FUNCTION reject_ticket_legacy_definition_binding_mutation() RETURNS trigger
 LANGUAGE plpgsql AS $$ BEGIN
 RAISE EXCEPTION 'legacy ticket definition binding receipts are immutable' USING ERRCODE='23514';
 END $$;

CREATE TRIGGER ticket_legacy_definition_binding_immutable
 BEFORE UPDATE OR DELETE OR TRUNCATE ON ticket_legacy_definition_bindings
 FOR EACH STATEMENT EXECUTE FUNCTION reject_ticket_legacy_definition_binding_mutation();

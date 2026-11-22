-- Stop new writes first. Only untouched cutover baselines can be discarded.
-- Keep explicit releases and bound ticket rules for controlled recovery.
DO $$ BEGIN
 LOCK TABLE ticket_types, tickets, ticket_type_definitions IN ACCESS EXCLUSIVE MODE;
 IF EXISTS(SELECT 1 FROM tickets WHERE definition_version IS NOT NULL)
    OR EXISTS(SELECT 1 FROM ticket_type_definitions d JOIN ticket_types t ON t.code=d.type_code
              WHERE d.published_by IS NOT NULL OR t.version<>d.source_draft_revision) THEN
  RAISE EXCEPTION 'D01 releases, bound tickets or changed cutover rules exist; retain definitions and bindings before rollback';
 END IF;
 ALTER TABLE tickets DROP CONSTRAINT IF EXISTS tickets_definition_fk;
 DROP INDEX IF EXISTS idx_tickets_definition_status;
 ALTER TABLE tickets DROP COLUMN IF EXISTS definition_version;
 ALTER TABLE ticket_types DROP CONSTRAINT IF EXISTS ticket_types_published_fk;
 ALTER TABLE ticket_types DROP COLUMN IF EXISTS current_published_version;
 DROP TABLE IF EXISTS ticket_type_definitions;
 DROP FUNCTION IF EXISTS ticket_definition_snapshot_hash(JSONB);
 DROP FUNCTION IF EXISTS reject_ticket_definition_mutation();
 DROP TABLE IF EXISTS ticket_field_identities;
END $$;

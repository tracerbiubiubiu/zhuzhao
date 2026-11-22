DO $$ BEGIN
 LOCK TABLE ticket_legacy_definition_bindings IN ACCESS EXCLUSIVE MODE;
 IF EXISTS (SELECT 1 FROM ticket_legacy_definition_bindings) THEN
  RAISE EXCEPTION 'retain legacy ticket definition binding receipts before rollback';
 END IF;
 DROP TABLE ticket_legacy_definition_bindings;
 DROP FUNCTION reject_ticket_legacy_definition_binding_mutation();
END $$;

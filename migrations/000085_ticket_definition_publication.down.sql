-- Preserve operation replay and audit facts after any successful publication.
DO $$ BEGIN
    LOCK TABLE ticket_definition_publication_receipts IN ACCESS EXCLUSIVE MODE;
    IF EXISTS (SELECT 1 FROM ticket_definition_publication_receipts) THEN
        RAISE EXCEPTION 'retain ticket definition publication receipts before rollback';
    END IF;
    DROP TABLE ticket_definition_publication_receipts;
    DROP TABLE ticket_definition_publication_plans;
END $$;

DELETE FROM casbin_rule
WHERE ptype = 'p' AND (v1, v2) IN (
    ('/api/v1/ticket-types/definitions/plan', 'POST'),
    ('/api/v1/ticket-types/definitions/publish', 'POST'),
    ('/api/v1/ticket-types/:code/published', 'GET')
);

DELETE FROM menu_apis a USING menus m
WHERE a.menu_id = m.id AND (
    m.code = 'ticket_definition_publish_btn'
    OR (m.code IN ('ticket_create_btn', 'ticket_type_manage')
        AND (a.api_path, a.api_method) = ('/api/v1/ticket-types/:code/published', 'GET'))
    OR (m.code = 'ticket_type_write_btn'
        AND (a.api_path, a.api_method) = ('/api/v1/ticket-types/:code/fields', 'GET'))
);

DELETE FROM role_menus rm USING menus m
WHERE rm.menu_id = m.id AND m.code = 'ticket_definition_publish_btn';
DELETE FROM menus WHERE code = 'ticket_definition_publish_btn';

-- Restore the v84 catalog binding before deciding which grants still have a source.
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT id, '/api/v1/ticket-types/:code/fields', 'GET'
FROM menus WHERE code = 'ticket_list' AND deleted_at IS NULL
ON CONFLICT DO NOTHING;

-- A grant derived from the v85 write button must have a remaining v84 source.
DELETE FROM casbin_rule c USING roles r
WHERE c.ptype = 'p' AND (c.v1, c.v2) = ('/api/v1/ticket-types/:code/fields', 'GET')
  AND c.v0 = 'role::' || r.code AND r.deleted_at IS NULL
  AND EXISTS (
      SELECT 1 FROM role_menus rm
      JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_type_write_btn' AND m.deleted_at IS NULL
      WHERE rm.role_id = r.id
  )
  AND NOT EXISTS (
      SELECT 1 FROM role_menus rm
      JOIN menus m ON m.id = rm.menu_id AND m.deleted_at IS NULL
      JOIN menu_apis a ON a.menu_id = m.id
      WHERE rm.role_id = r.id AND (a.api_path, a.api_method) = (c.v1, c.v2)
  );

-- Restore only prior rows whose original role still owns the list binding.
INSERT INTO casbin_rule (id, ptype, v0, v1, v2, v3, v4, v5)
SELECT revoked.rule_id, 'p', revoked.subject, '/api/v1/ticket-types/:code/fields', 'GET', revoked.v3, revoked.v4, revoked.v5
FROM ticket_definition_draft_policy_revocations revoked
JOIN roles r ON r.id = revoked.role_id AND r.deleted_at IS NULL AND 'role::' || r.code = revoked.subject
WHERE EXISTS (
    SELECT 1 FROM role_menus rm
    JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_list' AND m.deleted_at IS NULL
    WHERE rm.role_id = r.id
)
ON CONFLICT DO NOTHING;

DROP TABLE ticket_definition_draft_policy_revocations;

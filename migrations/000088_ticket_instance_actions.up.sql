-- The action contract uses the same object-read audience as ticket detail.
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT id, '/api/v1/tickets/:id/actions', 'GET'
FROM menus WHERE code = 'ticket_list' AND deleted_at IS NULL
ON CONFLICT DO NOTHING;

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, a.api_path, a.api_method
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.deleted_at IS NULL
JOIN menu_apis a ON a.menu_id = m.id
WHERE r.deleted_at IS NULL AND m.code = 'ticket_list'
  AND (a.api_path, a.api_method) = ('/api/v1/tickets/:id/actions', 'GET')
ON CONFLICT DO NOTHING;

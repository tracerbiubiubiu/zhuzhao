DELETE FROM menu_apis a USING menus m
WHERE a.menu_id = m.id AND m.code = 'ticket_list'
  AND (a.api_path, a.api_method) = ('/api/v1/tickets/:id/actions', 'GET');

DELETE FROM casbin_rule c
WHERE c.ptype = 'p' AND (c.v1, c.v2) = ('/api/v1/tickets/:id/actions', 'GET')
  AND EXISTS (
    SELECT 1 FROM roles r
    JOIN role_menus rm ON rm.role_id = r.id
    JOIN menus m ON m.id = rm.menu_id AND m.code = 'ticket_list'
    WHERE c.v0 = 'role::' || r.code
      AND NOT EXISTS (
        SELECT 1 FROM role_menus other
        JOIN menus source ON source.id = other.menu_id AND source.deleted_at IS NULL
        JOIN menu_apis a ON a.menu_id = source.id
        WHERE other.role_id = r.id AND (a.api_path, a.api_method) = (c.v1, c.v2)
      )
  );

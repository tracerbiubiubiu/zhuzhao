DELETE FROM menu_apis WHERE (api_path = '/api/v1/notifications/inbox' AND api_method = 'GET')
   OR (api_path = '/api/v1/notifications/read' AND api_method = 'POST')
   OR (api_path = '/api/v1/notifications/read-all' AND api_method = 'POST');
DELETE FROM menus WHERE code = 'message_center' AND deleted_at IS NULL;
DROP TABLE IF EXISTS notifications;

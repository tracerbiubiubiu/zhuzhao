-- 000077 down: 写端点回挂页面行、按钮行摘除（回到 000034/000059 形态）。
-- ② 补的按钮绑定与 admin/superadmin 既有绑定不可区分，保留（按钮行无 apis 即惰性）；casbin 不逆向重建。
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, 'POST'
FROM menus m JOIN (VALUES
    ('system_dict',         '/api/v1/dicts'),
    ('system_dict',         '/api/v1/dicts/update'),
    ('system_dict',         '/api/v1/dicts/delete'),
    ('system_dict',         '/api/v1/dict-items'),
    ('system_dict',         '/api/v1/dict-items/update'),
    ('system_dict',         '/api/v1/dict-items/delete'),
    ('system_notification', '/api/v1/notifications'),
    ('system_notification', '/api/v1/notifications/update'),
    ('system_notification', '/api/v1/notifications/delete'),
    ('system_notification', '/api/v1/message-templates'),
    ('system_notification', '/api/v1/message-templates/update'),
    ('system_notification', '/api/v1/message-templates/delete'),
    ('system_notification', '/api/v1/message-rules'),
    ('system_notification', '/api/v1/message-rules/delete')
) AS v(page_code, api_path) ON m.code = v.page_code AND m.deleted_at IS NULL
WHERE NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = 'POST');

DELETE FROM menu_apis
WHERE api_method = 'POST'
  AND menu_id IN (SELECT id FROM menus WHERE code IN ('dict_write_btn', 'notification_write_btn'));

-- 000077: 字典/通知配置写端点迁按钮行（G-26 对账收口——W1 B 案「页面=读 API、写端点挂按钮」漏项）
-- 现状：000034 把字典 CRUD×6 挂 system_dict 页面行、000059 把消息管理 8 端点重绑 system_notification 页面行；
--       对应按钮行 dict_write_btn(dict:manage) / notification_write_btn(notification:manage) 建行即空绑（apis=0），
--       前端写按钮按这两个码渲染（dict 页 v-permission）而 API 面却随页面行放行——两页当前仅绑 admin/superadmin
--       （预设不含）故为潜伏越权；任何自建角色被分配页面即得全写。
-- 处置同 000031/000075：写端点改挂按钮行 + 页面行摘除（GET 保留）+ intent-preserving + 非管理角色残留写 casbin 清除。

-- ① 写端点挂按钮行
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, 'POST'
FROM menus m JOIN (VALUES
    ('dict_write_btn',         '/api/v1/dicts'),
    ('dict_write_btn',         '/api/v1/dicts/update'),
    ('dict_write_btn',         '/api/v1/dicts/delete'),
    ('dict_write_btn',         '/api/v1/dict-items'),
    ('dict_write_btn',         '/api/v1/dict-items/update'),
    ('dict_write_btn',         '/api/v1/dict-items/delete'),
    ('notification_write_btn', '/api/v1/notifications'),
    ('notification_write_btn', '/api/v1/notifications/update'),
    ('notification_write_btn', '/api/v1/notifications/delete'),
    ('notification_write_btn', '/api/v1/message-templates'),
    ('notification_write_btn', '/api/v1/message-templates/update'),
    ('notification_write_btn', '/api/v1/message-templates/delete'),
    ('notification_write_btn', '/api/v1/message-rules'),
    ('notification_write_btn', '/api/v1/message-rules/delete')
) AS v(btn_code, api_path) ON m.code = v.btn_code AND m.deleted_at IS NULL
WHERE NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = 'POST');

-- ② intent-preserving（000031 ② 同款）：非管理角色若经页面绑定且 casbin 已含该写路径 → 补对应按钮绑定，
--    防下一次 AssignMenus 重存时写权限静默消失（当前库 0 行，结构性防御）
INSERT INTO role_menus (role_id, menu_id)
SELECT DISTINCT rm.role_id, btn.id
FROM role_menus rm
JOIN menus page ON page.id = rm.menu_id AND page.code IN ('system_dict', 'system_notification') AND page.deleted_at IS NULL
JOIN roles r ON r.id = rm.role_id AND r.deleted_at IS NULL AND r.code NOT IN ('admin', 'superadmin')
JOIN menus btn ON btn.deleted_at IS NULL
             AND btn.code = CASE page.code WHEN 'system_dict' THEN 'dict_write_btn' ELSE 'notification_write_btn' END
JOIN menu_apis ma ON ma.menu_id = page.id AND ma.api_method = 'POST'
JOIN casbin_rule cr ON cr.ptype = 'p' AND cr.v0 = 'role::' || r.code AND cr.v1 = ma.api_path AND cr.v2 = 'POST'
ON CONFLICT (role_id, menu_id) DO NOTHING;

-- ③ 页面行摘除写端点（GET 保留——页面=读 API）
DELETE FROM menu_apis
WHERE api_method = 'POST'
  AND menu_id IN (SELECT id FROM menus WHERE code IN ('system_dict', 'system_notification'));

-- ④ 非管理角色残留写 casbin 清除（已按 ② 补按钮绑定者由下一次 AssignMenus 重算恢复；admin/superadmin 走通配）
DELETE FROM casbin_rule
WHERE ptype = 'p' AND v0 NOT IN ('role::admin', 'role::superadmin') AND v2 = 'POST'
  AND v1 IN ('/api/v1/dicts', '/api/v1/dicts/update', '/api/v1/dicts/delete',
             '/api/v1/dict-items', '/api/v1/dict-items/update', '/api/v1/dict-items/delete',
             '/api/v1/notifications', '/api/v1/notifications/update', '/api/v1/notifications/delete',
             '/api/v1/message-templates', '/api/v1/message-templates/update', '/api/v1/message-templates/delete',
             '/api/v1/message-rules', '/api/v1/message-rules/delete');

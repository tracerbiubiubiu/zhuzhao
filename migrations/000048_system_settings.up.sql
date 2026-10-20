-- 000048: system_settings KV 表 + 设置管理页/按钮种子（phase5 S5 B5-6 配置化——05 号 §3/A.4 #6）
--
-- 三层结构（05 号 §3）：① config 出厂默认（boot 兜底）→ ② 本表运行时覆盖 → ③ 管理面 UI。
-- 存储限制为首个消费者；审计保留期管理端点为已登记的第二个消费者（复用同一面）。
-- 按 key 编辑层级 min_editor_tier 在代码目录（catalog），不进 DB——语义进代码纪律（§25.3 同构）。
--
-- 菜单：system_setting 页面行（GET 面挂此行）+ system_setting_update 按钮（POST 面）。
-- 权限码 system:setting:update——admin/superadmin 均授（tier 分层在服务端校验，不靠菜单裁剪）。

CREATE TABLE system_settings (
    key         VARCHAR(100) PRIMARY KEY,
    value       JSONB       NOT NULL,
    updated_by  BIGINT      NOT NULL REFERENCES users(id),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- 菜单：设置管理页（system 子目录尾位——sort_order 取现有 system 子菜单最大 +1）
INSERT INTO menus (parent_id, menu_type, code, name, path, component, permission, sort_order, icon, visible, is_system)
SELECT m.id, 2, 'system_setting', '系统设置', '/system/setting', 'system/setting/index',
       '', 99, 'setting', true, true
FROM menus m WHERE m.code = 'system'
ON CONFLICT DO NOTHING;

INSERT INTO menus (parent_id, menu_type, code, name, path, component, permission, sort_order, icon, visible, is_system)
SELECT m.id, 3, 'system_setting_update', '修改设置', '', '', 'system:setting:update', 1, '', true, true
FROM menus m WHERE m.code = 'system_setting'
ON CONFLICT DO NOTHING;

-- menu_apis：GET 挂页面行（读=页面权限）；POST 挂按钮行（写=按钮权限码）
INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m JOIN (VALUES
    ('system_setting',        '/api/v1/system/settings', 'GET'),
    ('system_setting_update', '/api/v1/system/settings/update', 'POST')
) AS v(menu_code, api_path, api_method) ON m.code = v.menu_code
ON CONFLICT DO NOTHING;

-- role_menus：admin + superadmin 全量授（对齐 000031 惯例——tier 分层在服务端 catalog 校验）
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id FROM roles r CROSS JOIN menus m
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL
  AND m.code IN ('system_setting', 'system_setting_update')
ON CONFLICT DO NOTHING;

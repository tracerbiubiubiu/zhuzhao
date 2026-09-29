-- P4-3 字典/业务枚举运行时化（边界：只做业务枚举/运维参数，不碰权限策略面——
-- 平台策略=逻辑在代码，00 §2.1 雷达拍板）。
-- 场景=工单自定义字段选项等无处可挂的业务枚举；gva 形态（类型/项两级+启停+按 type 拉取）。
-- 菜单显式高位 id（90003/90004）——roundtrip 零漂移口径（000033 先例）。
CREATE TABLE IF NOT EXISTS dict_types (
    id         BIGSERIAL PRIMARY KEY,
    code       VARCHAR(64)  NOT NULL UNIQUE,
    name       VARCHAR(128) NOT NULL,
    enabled    BOOLEAN      NOT NULL DEFAULT true,
    remark     VARCHAR(255) NOT NULL DEFAULT '',
    version    BIGINT       NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS dict_items (
    id         BIGSERIAL PRIMARY KEY,
    type_code  VARCHAR(64)  NOT NULL,
    code       VARCHAR(64)  NOT NULL,
    label      VARCHAR(128) NOT NULL,
    sort_order INT          NOT NULL DEFAULT 0,
    enabled    BOOLEAN      NOT NULL DEFAULT true,
    remark     VARCHAR(255) NOT NULL DEFAULT '',
    version    BIGINT       NOT NULL DEFAULT 1,
    created_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT fk_dict_items_type FOREIGN KEY (type_code) REFERENCES dict_types (code) ON DELETE CASCADE,
    CONSTRAINT uq_dict_items_type_code UNIQUE (type_code, code)
);
CREATE INDEX IF NOT EXISTS idx_dict_items_type ON dict_items (type_code, sort_order, code);

-- 管理面：页面（dict:read）+写按钮（dict:manage）——B 案词表（页面=GET，写挂按钮行）
INSERT INTO menus (id, code, name, parent_id, menu_type, path, component, icon, permission, sort_order, visible, is_system)
VALUES (90003, 'system_dict', '字典管理', NULL, 2, '/system/dict', 'system/dict/index', 'dict', 'dict:read', 7, true, true)
ON CONFLICT DO NOTHING;

INSERT INTO menus (id, parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT 90004, p.id, 'dict_write_btn', '字典写', 3, 'dict:manage', 1, true
FROM menus p WHERE p.code = 'system_dict'
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m JOIN (VALUES
    ('system_dict', '/api/v1/dicts',              'GET'),
    ('system_dict', '/api/v1/dicts',              'POST'),
    ('system_dict', '/api/v1/dicts/update',       'POST'),
    ('system_dict', '/api/v1/dicts/delete',       'POST'),
    ('system_dict', '/api/v1/dict-items',         'GET'),
    ('system_dict', '/api/v1/dict-items',         'POST'),
    ('system_dict', '/api/v1/dict-items/update',  'POST'),
    ('system_dict', '/api/v1/dict-items/delete',  'POST'),
    -- 消费面：按 type 拉取启用项（业务表单选项——参数路由 :code 形态，同 al :typeName 先例）
    ('system_dict', '/api/v1/dicts/:code/items',   'GET')
) AS v(menu_code, api_path, api_method) ON m.code = v.menu_code
ON CONFLICT DO NOTHING;

-- role_menus 补绑（000031 ⑥ 同款语义）：新菜单（含 000033 的 system_notification/
-- notification_write_btn——该批漏绑，此处一并补）须绑 admin/superadmin 才对
-- GetUserMenus 可见（INNER JOIN 无通配旁路）。
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id
FROM roles r
JOIN menus m ON m.id IN (90001, 90002, 90003, 90004)
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM role_menus rm WHERE rm.role_id = r.id AND rm.menu_id = m.id)
ON CONFLICT DO NOTHING;

-- role_menus 补绑（000031 ⑥ 同款语义）：新菜单（含 000033 的 system_notification/
-- notification_write_btn——该批漏绑，此处一并补）须绑 admin/superadmin 才对
-- GetUserMenus 可见（INNER JOIN 无通配旁路）。
INSERT INTO role_menus (role_id, menu_id)
SELECT r.id, m.id
FROM roles r
JOIN menus m ON m.id IN (90001, 90002, 90003, 90004)
WHERE r.code IN ('admin', 'superadmin') AND r.deleted_at IS NULL
  AND NOT EXISTS (SELECT 1 FROM role_menus rm WHERE rm.role_id = r.id AND rm.menu_id = m.id)
ON CONFLICT DO NOTHING;

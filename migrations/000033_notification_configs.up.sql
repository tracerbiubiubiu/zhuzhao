-- P4-2 通知通道本体（2026-09-21 拍板：webhook 优先；配置持久化+管理 API 面，
-- 配置页触发驱动后补）。场景=taskrunner 死信告警（不依赖工单解封）+
-- 工单状态流转/待办（依赖事件消费面——独立连号随触发补）。
--
-- 菜单过渡态：system_notification visible=false（配置页后补，页面就绪置 true
-- 即现——menu_apis 先行绑定防 catalog missing_binding 拒启）；B 案词表：
-- 页面行=GET，三写挂 notification_write_btn（notification:manage 码）。
CREATE TABLE IF NOT EXISTS notification_configs (
    id           BIGSERIAL PRIMARY KEY,
    code         VARCHAR(64)  NOT NULL UNIQUE,
    name         VARCHAR(128) NOT NULL,
    channel      VARCHAR(32)  NOT NULL DEFAULT 'webhook', -- 渠道枚举（抽象接口配置化——首版仅 webhook）
    webhook_url  TEXT         NOT NULL,                   -- channel=webhook 必填（服务端校验）
    enabled      BOOLEAN      NOT NULL DEFAULT true,
    created_by   BIGINT,
    version      BIGINT       NOT NULL DEFAULT 1,         -- 乐观锁（管理面并发改配置）
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notification_configs_enabled ON notification_configs (enabled) WHERE enabled;

-- 显式高位 id（90001/90002，远离自增段）：down 删行后 up 重插同值——
-- roundtrip 明细 diff 零漂移（BIGSERIAL 重插 id 会变，27 批往返门禁口径）
INSERT INTO menus (id, code, name, parent_id, menu_type, path, component, icon, permission, sort_order, visible, is_system)
VALUES (90001, 'system_notification', '通知配置', NULL, 2, '/system/notification', 'system/notification/index', 'notification', 'notification:read', 6, false, true)
ON CONFLICT DO NOTHING;

-- 按钮码（前端显隐；路由级授权由 menu_apis → casbin 承担——同 000024 形态）
INSERT INTO menus (id, parent_id, code, name, menu_type, permission, sort_order, is_system)
SELECT 90002, p.id, 'notification_write_btn', '通知配置写', 3, 'notification:manage', 1, true
FROM menus p WHERE p.code = 'system_notification'
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method FROM menus m JOIN (VALUES
    ('system_notification', '/api/v1/notifications',        'GET'),
    ('system_notification', '/api/v1/notifications',        'POST'),
    ('system_notification', '/api/v1/notifications/update', 'POST'),
    ('system_notification', '/api/v1/notifications/delete', 'POST')
) AS v(menu_code, api_path, api_method) ON m.code = v.menu_code
ON CONFLICT DO NOTHING;

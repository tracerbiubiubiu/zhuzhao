-- S6 二期 B6-7 站内消息中心（06 号 §6/附录 A.6 DDL）：
-- notifications = 事件快照（读过即完——GitHub Notifications 语义）；
-- recipient/biz 深链/created_at + 行级 read_at（第一档）。
-- 保留策略：N 天封顶（cleanup 骑 CronScheduler——复用 message_log 经验）。
CREATE TABLE IF NOT EXISTS notifications (
    id           BIGSERIAL PRIMARY KEY,
    recipient_id BIGINT NOT NULL REFERENCES users(id),
    biz_type     VARCHAR(30) NOT NULL,
    biz_id       BIGINT,
    title        VARCHAR(255) NOT NULL,          -- 渲染后摘要（入站即渲染——读面零渲染逻辑）
    link         VARCHAR(255),                   -- 深链路由（payload 路由既定）
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    read_at      TIMESTAMPTZ                     -- 行级已读（第一档——单条/全部已读）
);
CREATE INDEX IF NOT EXISTS idx_notifications_recipient ON notifications (recipient_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_notifications_unread ON notifications (recipient_id)
    WHERE read_at IS NULL;

-- 消息中心菜单（页面行——铃铛点入/消息中心页；沿用 system_notification 族或独立码？
-- 独立 inbox:read 码=最小面起步，页面+端点绑定）。
INSERT INTO menus (code, name, parent_id, menu_type, path, component, icon, permission, sort_order, is_system)
VALUES ('message_center', '消息中心', NULL, 2, '/messages', 'message/center/index', 'message', 'inbox:read', 7, true)
ON CONFLICT DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT m.id, v.api_path, v.api_method
FROM menus m, (VALUES
    ('message_center', '/api/v1/notifications/inbox', 'GET'),
    ('message_center', '/api/v1/notifications/read', 'POST'),
    ('message_center', '/api/v1/notifications/read-all', 'POST')
) AS v(menu_code, api_path, api_method)
WHERE m.code = v.menu_code AND m.deleted_at IS NULL
AND NOT EXISTS (SELECT 1 FROM menu_apis ma WHERE ma.menu_id = m.id AND ma.api_path = v.api_path AND ma.api_method = v.api_method);

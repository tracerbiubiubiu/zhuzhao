-- S6 B6-4 用户通知偏好（06 号 §5.1——per-user opt-out/免打扰）
CREATE TABLE IF NOT EXISTS notification_optouts (
    id           BIGSERIAL PRIMARY KEY,
    user_id      BIGINT      NOT NULL REFERENCES users(id),
    biz_type     VARCHAR(32) NOT NULL DEFAULT 'ticket',
    event_action VARCHAR(64) NOT NULL DEFAULT '*', -- '*' = 全域静默（免打扰）
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, biz_type, event_action)
);
CREATE INDEX IF NOT EXISTS idx_notification_optouts_user ON notification_optouts (user_id, biz_type, event_action);

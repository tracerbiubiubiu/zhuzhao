CREATE TABLE notification_http_profiles (
    profile_ref TEXT PRIMARY KEY,
    binding_sha256 TEXT NOT NULL
);

CREATE TABLE notification_deliveries (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL,
    source_event_id TEXT NOT NULL,
    rule_id TEXT NOT NULL,
    rule_version BIGINT NOT NULL CHECK (rule_version > 0),
    recipient_user_id BIGINT NOT NULL,
    channel TEXT NOT NULL,
    semantic_key TEXT NOT NULL,
    ticket_id BIGINT NOT NULL,
    ticket_revision BIGINT NOT NULL,
    operation_id TEXT NOT NULL,
    profile_ref TEXT NOT NULL,
    payload JSONB NOT NULL,
    payload_sha256 TEXT NOT NULL,
    state TEXT NOT NULL CHECK (state IN ('pending','sending','accepted','delivered','failed','unknown','suppressed')),
    revision BIGINT NOT NULL DEFAULT 0,
    sends INTEGER NOT NULL DEFAULT 0,
    attempts INTEGER NOT NULL DEFAULT 0,
    next_at TIMESTAMPTZ,
    lease_until TIMESTAMPTZ,
    expires_at TIMESTAMPTZ NOT NULL,
    provider_receipt_ref TEXT NOT NULL DEFAULT '',
    provider_observed_at TIMESTAMPTZ,
    reason TEXT NOT NULL DEFAULT '',
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (source, source_event_id, rule_id, rule_version, recipient_user_id, channel, semantic_key)
);

CREATE INDEX notification_delivery_ready ON notification_deliveries
    (profile_ref, next_at, lease_until) WHERE state IN ('pending','sending','accepted','unknown')
    AND (next_at IS NOT NULL OR lease_until IS NOT NULL);

CREATE TABLE notification_delivery_attempts (
    delivery_id TEXT NOT NULL REFERENCES notification_deliveries(id) ON DELETE CASCADE,
    attempt INTEGER NOT NULL,
    mode TEXT NOT NULL CHECK (mode IN ('send','query')),
    started_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    finished_at TIMESTAMPTZ,
    result TEXT NOT NULL DEFAULT '',
    PRIMARY KEY (delivery_id, attempt)
);

CREATE TABLE notification_provider_events (
    profile_ref TEXT NOT NULL,
    event_id TEXT NOT NULL,
    delivery_id TEXT NOT NULL REFERENCES notification_deliveries(id) ON DELETE CASCADE,
    receipt JSONB NOT NULL,
    PRIMARY KEY (profile_ref, event_id)
);

CREATE TABLE notification_review_notices (
    ticket_id BIGINT NOT NULL,
    ticket_revision BIGINT NOT NULL,
    recipient_id BIGINT NOT NULL,
    operation_id TEXT NOT NULL,
    notification_id BIGINT UNIQUE REFERENCES notifications(id) ON DELETE SET NULL,
    delivery_id TEXT REFERENCES notification_deliveries(id) ON DELETE SET NULL,
    PRIMARY KEY (ticket_id, ticket_revision, recipient_id)
);
CREATE INDEX notification_review_notices_recipient ON notification_review_notices (recipient_id) WHERE notification_id IS NOT NULL;

-- A preexisting or recreated template with the same code retains its own identity.
ALTER TABLE message_templates ADD COLUMN review_http_mock_seed BOOLEAN NOT NULL DEFAULT false;
INSERT INTO message_templates (code, name, channel, biz_type, subject, body, review_http_mock_seed)
VALUES ('ticket_review_http_mock', '工单待复核（模拟外送）', 'webhook', 'ticket', '有工单待复核', '请登录工作台查看待复核工单。', true)
ON CONFLICT (code) DO NOTHING;

INSERT INTO menu_apis (menu_id, api_path, api_method)
SELECT id, '/api/v1/message-templates/preview', 'POST'
FROM menus WHERE code='notification_write_btn' AND deleted_at IS NULL;

INSERT INTO casbin_rule (ptype, v0, v1, v2)
SELECT DISTINCT 'p', 'role::' || r.code, '/api/v1/message-templates/preview', 'POST'
FROM roles r
JOIN role_menus rm ON rm.role_id = r.id
JOIN menus m ON m.id = rm.menu_id AND m.code = 'notification_write_btn' AND m.deleted_at IS NULL
WHERE r.deleted_at IS NULL
  AND NOT EXISTS (
    SELECT 1 FROM casbin_rule c
    WHERE c.ptype = 'p' AND c.v0 = 'role::' || r.code
      AND c.v1 = '/api/v1/message-templates/preview' AND c.v2 = 'POST'
  );

-- 000083 C05：DB 安全代际 + 私有 operation/intent 存储
-- 设计源：docs/phase6/foundation-iam-module-detail.md §4.2/§4.4。
-- session_epoch ≥ 1（DB 安全代际写主）；session_projection_epoch ∈ [0, session_epoch]
-- 只表示该代 Redis 投影已确认，不代表业务侧全部生效。

ALTER TABLE users
    ADD COLUMN session_epoch BIGINT NOT NULL DEFAULT 1,
    ADD COLUMN session_projection_epoch BIGINT NOT NULL DEFAULT 0;

ALTER TABLE users
    ADD CONSTRAINT users_session_epoch_ck CHECK (session_epoch >= 1);
ALTER TABLE users
    ADD CONSTRAINT users_session_projection_epoch_ck
    CHECK (session_projection_epoch >= 0 AND session_projection_epoch <= session_epoch);

-- 私有操作回执（F021：同 K 最多执行一次；口令/token 不落正文，
-- input_fingerprint 只覆盖非秘密投影 hash）。
CREATE TABLE user_session_operations (
    id            BIGSERIAL PRIMARY KEY,
    operation_id  TEXT        NOT NULL UNIQUE,
    namespace     TEXT        NOT NULL DEFAULT 'http', -- http | cli | bootstrap
    command       TEXT        NOT NULL,                -- user.status / user.delete / user.reset_password / auth.change_password / user.create
    target_user_id BIGINT     NOT NULL REFERENCES users(id),
    actor_user_id BIGINT      NOT NULL,
    expected_epoch BIGINT     NOT NULL,                -- 首次提交时冻结的目标代际预期
    input_fingerprint TEXT    NOT NULL DEFAULT '',     -- 非秘密绑定投影 sha256（不含口令）
    status        TEXT        NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'completed', 'rolled_back')),
    result_epoch  BIGINT,                              -- 完成后的目标代际（completed 时非空）
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK (expected_epoch >= 1),
    CHECK ((status = 'completed' AND result_epoch IS NOT NULL AND result_epoch >= 1)
        OR (status <> 'completed' AND result_epoch IS NULL)),
    UNIQUE (operation_id, target_user_id, result_epoch)
);
CREATE INDEX idx_user_session_operations_target
    ON user_session_operations (target_user_id, created_at DESC);

-- 会话投影同步意图（恢复必需：DB 提交后 Redis 失败可跨进程续投）。
CREATE TABLE session_projection_intents (
    id            BIGSERIAL PRIMARY KEY,
    operation_id  TEXT        NOT NULL UNIQUE,
    user_id       BIGINT      NOT NULL,
    target_epoch  BIGINT      NOT NULL,
    desired_active BOOLEAN    NOT NULL,
    reason        TEXT        NOT NULL DEFAULT '',
    lease_token   TEXT        NOT NULL DEFAULT '',
    lease_expires_at TIMESTAMPTZ,
    fence         BIGINT      NOT NULL DEFAULT 0,
    attempts      INT         NOT NULL DEFAULT 0,
    last_error    TEXT        NOT NULL DEFAULT '',
    status        TEXT        NOT NULL DEFAULT 'pending'
                  CHECK (status IN ('pending', 'confirmed', 'superseded')),
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (user_id, target_epoch),
    FOREIGN KEY (operation_id, user_id, target_epoch)
        REFERENCES user_session_operations (operation_id, target_user_id, result_epoch)
);
CREATE INDEX idx_session_projection_intents_pending
    ON session_projection_intents (id) WHERE status = 'pending';
CREATE INDEX idx_session_projection_intents_user
    ON session_projection_intents (user_id, target_epoch DESC);

-- 存量初始化（可重入）：为全部存量账号生成初始代际 1 与待投影意图（epoch=1/0
-- 由列默认补齐），停用账号不被激活（desired_active 按当前 status 冻结）。
-- bootstrap:<uid> 稳定操作号保证重入不重复。
INSERT INTO user_session_operations
    (operation_id, namespace, command, target_user_id, actor_user_id, expected_epoch, status, result_epoch)
SELECT 'bootstrap:' || u.id, 'bootstrap', 'user.create', u.id, u.id, 1, 'completed', 1
FROM users u
WHERE u.session_epoch = 1 AND NOT EXISTS (
    SELECT 1 FROM user_session_operations o WHERE o.target_user_id = u.id AND o.result_epoch = 1
) ON CONFLICT (operation_id) DO NOTHING;

INSERT INTO session_projection_intents
    (operation_id, user_id, target_epoch, desired_active, reason, status)
SELECT o.operation_id, u.id, 1, (u.status = 1 AND u.deleted_at IS NULL), 'bootstrap-backfill', 'pending'
FROM users u JOIN user_session_operations o ON o.operation_id = 'bootstrap:' || u.id
WHERE u.session_epoch = 1 AND NOT EXISTS (
    SELECT 1 FROM session_projection_intents i WHERE i.user_id = u.id AND i.target_epoch = 1
) ON CONFLICT DO NOTHING;

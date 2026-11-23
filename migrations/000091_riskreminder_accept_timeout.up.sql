-- Phase6 B2 item 8: first-acceptance timeout in-site reminder kernel storage.
-- Assignments are the dispatch-time facts (generation, assignee, reminder
-- params published at dispatch, acceptance deadline); intents are the
-- persistent pending/delivered/suppressed state whose dedup key follows the
-- D15 delivery tuple. Reminder policy itself lives with the dispatcher; this
-- schema only stores facts and executes them.
--
-- Ticket deletion is a hard delete, so reminder state is ticket-attached and
-- cascades with the ticket (same as ticket_comments/ticket_relations). The
-- notification_id column deliberately has no FK: notifications retention
-- cleanup must not be blocked by delivery receipts.

CREATE TABLE riskreminder_assignments (
    id            BIGSERIAL PRIMARY KEY,
    ticket_id     BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    generation    BIGINT NOT NULL CHECK (generation > 0),
    assignee_id   BIGINT NOT NULL REFERENCES users(id),
    dispatched_by BIGINT NOT NULL REFERENCES users(id),
    dispatched_at TIMESTAMPTZ NOT NULL,
    accept_due_at TIMESTAMPTZ NOT NULL,
    rule_id       TEXT NOT NULL CHECK (octet_length(rule_id) BETWEEN 1 AND 128),
    rule_version  BIGINT NOT NULL CHECK (rule_version > 0),
    accepted_at   TIMESTAMPTZ,
    accepted_by   BIGINT REFERENCES users(id),
    -- One row per dispatch round; re-recording the same generation is
    -- idempotent in the kernel and fact changes are a conflict.
    UNIQUE (ticket_id, generation),
    CHECK ((accepted_at IS NULL) = (accepted_by IS NULL))
);

CREATE TABLE riskreminder_intents (
    id             BIGSERIAL PRIMARY KEY,
    assignment_id  BIGINT NOT NULL REFERENCES riskreminder_assignments(id) ON DELETE CASCADE,
    ticket_id      BIGINT NOT NULL,
    generation     BIGINT NOT NULL,
    recipient_id   BIGINT NOT NULL REFERENCES users(id),
    due_at         TIMESTAMPTZ NOT NULL,
    status         VARCHAR(20) NOT NULL DEFAULT 'pending'
                       CHECK (status IN ('pending', 'delivered', 'suppressed')),
    -- D15 delivery tuple; the unique key gives persistent dedup across
    -- rescans and restarts (one slot per dispatch fact + rule version + recipient).
    source         TEXT NOT NULL CHECK (octet_length(source) BETWEEN 1 AND 64),
    source_event_id TEXT NOT NULL CHECK (octet_length(source_event_id) BETWEEN 1 AND 128),
    rule_id        TEXT NOT NULL CHECK (octet_length(rule_id) BETWEEN 1 AND 128),
    rule_version   BIGINT NOT NULL CHECK (rule_version > 0),
    channel        TEXT NOT NULL CHECK (octet_length(channel) BETWEEN 1 AND 32),
    semantic_key   TEXT NOT NULL CHECK (octet_length(semantic_key) BETWEEN 1 AND 128),
    UNIQUE (source, source_event_id, rule_id, rule_version, recipient_id, channel, semantic_key),
    -- Delivery receipt. No FK by design (see header): retention cleanup of
    -- notifications must stay independent.
    notification_id BIGINT,
    delivered_at    TIMESTAMPTZ,
    suppressed_at   TIMESTAMPTZ,
    suppress_reason TEXT CHECK (octet_length(suppress_reason) BETWEEN 1 AND 128),
    -- Recoverable failure bookkeeping: visible attempts on the original key.
    attempt_count     INT NOT NULL DEFAULT 0 CHECK (attempt_count >= 0),
    last_attempted_at TIMESTAMPTZ,
    last_error        TEXT,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CHECK ((status = 'delivered') = (notification_id IS NOT NULL AND delivered_at IS NOT NULL)),
    CHECK ((status = 'suppressed') = (suppressed_at IS NOT NULL AND suppress_reason IS NOT NULL))
);

-- Scan and delivery pick pending work by due time; the partial index keeps
-- the hot set (pending only) small as delivered history accumulates.
CREATE INDEX idx_riskreminder_intents_pending_due
    ON riskreminder_intents (due_at, id) WHERE status = 'pending';
CREATE INDEX idx_riskreminder_intents_ticket
    ON riskreminder_intents (ticket_id, id);

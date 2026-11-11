-- 000046: ticket_attachments 工单附件关联表（phase5 S5 B5-3，05 号 A.3/10 号 §3 蓝本）
--
-- 关联语义：tickets 删除级联清附件行（对象本体由 GC/对账扫描收）；
-- file_objects 不级联（对象生命周期独立于关联）；UNIQUE 防同对象重复绑同单。
-- B5-3 扩展注：建单前绑定（attachment_create 悬挂态）由 CreateTicket 同事务
-- 落绑本表（05 号 A.5 建单前链——契约变更五点查随批）。
CREATE TABLE ticket_attachments (
    id              BIGSERIAL PRIMARY KEY,
    ticket_id       BIGINT NOT NULL REFERENCES tickets(id) ON DELETE CASCADE,
    file_object_id  BIGINT NOT NULL REFERENCES file_objects(id),
    uploaded_by     BIGINT NOT NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    UNIQUE (ticket_id, file_object_id)
);

CREATE INDEX idx_ticket_attachments_ticket ON ticket_attachments (ticket_id);

-- 000014 down: 恢复 CASCADE。先让位（§5.3 规则 2）——清理悬空/空引用事件行
--（对应已物理删除的工单），否则 NOT NULL + 外键重建会失败。
DELETE FROM ticket_events WHERE ticket_id IS NULL;
DELETE FROM ticket_events te
WHERE NOT EXISTS (SELECT 1 FROM tickets tk WHERE tk.id = te.ticket_id);

ALTER TABLE ticket_events
    DROP CONSTRAINT IF EXISTS ticket_events_ticket_id_fkey;
ALTER TABLE ticket_events
    ALTER COLUMN ticket_id SET NOT NULL;
ALTER TABLE ticket_events
    ADD CONSTRAINT ticket_events_ticket_id_fkey
    FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE CASCADE;

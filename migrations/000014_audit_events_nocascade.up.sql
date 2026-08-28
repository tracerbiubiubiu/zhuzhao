-- 000014: 审计完整性（HC2，2c 收口审查发现）
-- 原 000010 的 ticket_events FK 为 ON DELETE CASCADE：工单物理删除（管理员操作，
-- 04 H-5 硬删例外）会连带销毁整条业务时间线，且无法补写 deleted 事件。
-- 修正：ticket_id 改可空 + ON DELETE SET NULL——
--   删单后事件行存活（ticket_id 悬空 = 审计语义「此工单已被删除」）；
--   Delete 服务在删除前写入 action='deleted' 事件（可追溯操作者）。
-- 现有事件查询均按 ticket_id 过滤、不 join tickets，悬空行无功能影响。
-- ticket_comments 保持 CASCADE（用户内容随单删）；操作级留痕在 audit_logs。
ALTER TABLE ticket_events
    DROP CONSTRAINT IF EXISTS ticket_events_ticket_id_fkey;
ALTER TABLE ticket_events
    ALTER COLUMN ticket_id DROP NOT NULL;
ALTER TABLE ticket_events
    ADD CONSTRAINT ticket_events_ticket_id_fkey
    FOREIGN KEY (ticket_id) REFERENCES tickets(id) ON DELETE SET NULL;

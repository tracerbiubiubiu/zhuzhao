-- 000028 down: 还原「方向性」唯一索引（P1-2）
-- 注意：up 的解重软删不可逆——本 down 仅切换索引形态，不复活被软删的历史重复行。
DROP INDEX IF EXISTS uq_ticket_relations_normalized;
CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_relations_pair
  ON ticket_relations(source_ticket_id, target_ticket_id, relation_type)
  WHERE deleted_at IS NULL;

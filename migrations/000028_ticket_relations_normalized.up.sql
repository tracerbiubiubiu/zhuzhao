-- 000028: 工单关联「规范化对」唯一索引（P1-2 反向判重并发修复）
-- SSOT: 并发审查 P1-2——uq_ticket_relations_pair(source,target,relation_type) 是
-- 方向性的，service 层 ExistsRelationBetween 预检在事务外，并发 A→B / B→A 双双通过
-- 预检并入库（真机 59/60 轮出现 2 行）。改为对「无序对 + 类型」建部分唯一索引，
-- 由 DB 兜底并发正确性（预检仅作良好 UX 的顺序重复早返回）。
--
-- 关键：必须先解重再建唯一索引，否则线上/本地历史已有的双向重复行会让建索引失败。
-- 解重策略：同一「规范化对 + 关联类型」只保留最小 id 行，其余软删让位
-- （部分唯一索引带 WHERE deleted_at IS NULL，软删后即不参与唯一性约束）。
-- 表已有 CHECK (source_ticket_id <> target_ticket_id)，故规范化键不会撞自环（A→A）。
-- 注意：软删不可逆——down 仅还原索引，不复活被本迁移软删的历史重复行。

UPDATE ticket_relations t SET deleted_at = NOW()
WHERE t.deleted_at IS NULL AND EXISTS (
  SELECT 1 FROM ticket_relations k
  WHERE k.deleted_at IS NULL AND k.relation_type = t.relation_type
    AND LEAST(k.source_ticket_id, k.target_ticket_id) = LEAST(t.source_ticket_id, t.target_ticket_id)
    AND GREATEST(k.source_ticket_id, k.target_ticket_id) = GREATEST(t.source_ticket_id, t.target_ticket_id)
    AND k.id < t.id);

DROP INDEX IF EXISTS uq_ticket_relations_pair;
CREATE UNIQUE INDEX IF NOT EXISTS uq_ticket_relations_normalized
  ON ticket_relations (LEAST(source_ticket_id, target_ticket_id),
                       GREATEST(source_ticket_id, target_ticket_id),
                       relation_type)
  WHERE deleted_at IS NULL;

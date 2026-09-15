-- 000029：补回 ticket_relations 的 source 前导索引（000028 规范化时删除的方向
-- 唯一索引中，source 单列访问路径随之消失——ExistsRelationBetween 的 OR 谓词与
-- ListRelations 的 source 臂退化为顺序扫描；表达式索引只覆盖 LEAST/GREATEST
-- 对称谓词）。部分索引与软删纪律一致（F-6）。
CREATE INDEX IF NOT EXISTS idx_ticket_relations_source
    ON ticket_relations (source_ticket_id) WHERE deleted_at IS NULL;

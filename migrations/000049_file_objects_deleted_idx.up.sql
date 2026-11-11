-- 000049: file_objects deleted 态部分索引（phase5 S5——双审计 F-A7 随手批）
--
-- GC 二扫（ListDeleted：WHERE status='deleted' ORDER BY id，610fbcf 起）无索引
-- 可用——idx_file_objects_gc 是 pending 部分索引不覆盖 deleted；表增长后每轮
-- cron 全表 seq scan。deleted 行生命周期短（GC 收敛即物理清除），索引维护成本
-- 极低，直接建收口（不挂触发驱动）。
CREATE INDEX IF NOT EXISTS idx_file_objects_deleted ON file_objects (id) WHERE status = 'deleted';

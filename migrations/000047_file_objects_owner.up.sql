-- 000047: file_objects 通用归属两可空列（phase5 S5 随行件——2026-10-20 规划研究回灌，05 号 A.3「随行件迁移」段）
--
-- 语义：owner_kind = 归属实体类型（ticket / user；后续 script_package / run / ingest / export 复用），
--       owner_id   = 归属实体 ID 文本（跨类型统一为 TEXT，不建 FK——对象生命周期独立于归属实体）。
-- 写侧随本迁移同批补：ticket 态 confirm / 建单落绑 填 ticket；avatar bind 填 user。
-- 零回填：存量行 NULL 即兼容（归属元数据非判定依据，GC/对账消费者届时按 NULL=未归属处理）。
-- 不预建索引（触发驱动：出现按归属列举/对账查询再加部分索引 WHERE owner_kind IS NOT NULL）。
--
-- 编号：A2 当天对账取号——B5-6（system_settings）未启动、本迁移先建故占 000047
-- （05 号 A.3 预写口径 000048 系假设 B5-6 先占 047；golang-migrate 不允许事后插入更低版本，
--   先建者必占下一号——B5-6 顺位 000048，05/11/review-11 口径同批改）。
ALTER TABLE file_objects
    ADD COLUMN IF NOT EXISTS owner_kind VARCHAR(30) NULL,
    ADD COLUMN IF NOT EXISTS owner_id   TEXT        NULL;

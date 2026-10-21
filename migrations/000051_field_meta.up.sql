-- 000051: ticket_type_fields 追加 field_meta JSONB 列（phase5 S1 B1-3——01 号附录 A.2 方案 b）
--
-- 承载 formula（计算字段公式）/validation（regex 之外的 min·max·length）/auto_fill（触发驱动）
-- 等字段级元数据。与 field_options（select 选项数组）分列——混载破坏渲染器/optionValues
-- 消费方（附录 A.2 方案 b 拍定，2026-10-16 外审对齐后采纳新列方案）。
-- 零回填：存量行 NULL 即兼容（无 formula 的字段行为不变）。
ALTER TABLE ticket_type_fields ADD COLUMN IF NOT EXISTS field_meta JSONB DEFAULT NULL;

-- S6 §5.3 human_key：人读编号列（TK-yyyymmdd-NNNN）+ 唯一索引
ALTER TABLE tickets ADD COLUMN IF NOT EXISTS human_key VARCHAR(64);
CREATE UNIQUE INDEX IF NOT EXISTS idx_tickets_human_key ON tickets (human_key) WHERE human_key IS NOT NULL;
